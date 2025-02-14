package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/rivo/tview"
)

const (
	configDir  = ".deepseek-tui"
	chatsDir   = "chats"
	configFile = "config.json"
)

var (
	chatsMutex sync.RWMutex
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatSession struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Messages  []Message `json:"messages"`
	FilePath  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type OllamaRequest struct {
	Model    string    `json:"model"`
	Prompt   string    `json:"prompt"`
	Stream   bool      `json:"stream"`
	Messages []Message `json:"messages"`
}

func ensureConfigDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(home, configDir, chatsDir)
	return os.MkdirAll(path, 0755)
}

func saveChat(session *ChatSession) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	if session.FilePath == "" {
		session.FilePath = filepath.Join(
			home,
			configDir,
			chatsDir,
			fmt.Sprintf("chat_%s.json", session.ID),
		)
	}

	data, err := json.MarshalIndent(session, "", " ")
	if err != nil {
		return err
	}

	return os.WriteFile(session.FilePath, data, 0644)
}

func loadChats() (map[string]*ChatSession, error) {
	if err := ensureConfigDir(); err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()

	chatsFile, err := os.ReadDir(filepath.Join(home, configDir, chatsDir))
	if err != nil {
		return nil, err
	}

	chats := make(map[string]*ChatSession)
	for _, f := range chatsFile {
		if f.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(home, configDir, chatsDir, f.Name()))
		if err != nil {
			return nil, err
		}

		var chat ChatSession
		if err := json.Unmarshal(data, &chat); err == nil {
			chat.FilePath = filepath.Join(home, configDir, chatsDir, f.Name())
			chats[chat.ID.String()] = &chat
		}
	}

	return chats, nil
}

func deleteChat(chat *ChatSession, chats map[string]*ChatSession) error {
	if chat.FilePath == "" {
		return fmt.Errorf("chat path not found")
	}

	if err := os.Remove(chat.FilePath); err != nil {
		return err
	}

	delete(chats, chat.ID.String())

	return nil
}

func updateChatDisplay(chatView *tview.TextView, chat *ChatSession) {
	chatView.Clear()

	for _, msg := range chat.Messages {
		color := "[white]"

		if msg.Role == "user" {
			color = "[green]"
		} else {
			color = "[blue]"
		}

		fmt.Fprintf(chatView, "%s%s:[-] %s\n", color, msg.Role, msg.Content)
	}

	chatView.ScrollToEnd()
}

func queryOllamaStream(messages []Message, callback func(string)) error {
	requestData := OllamaRequest{
		Model:    "deepseek-r1",
		Stream:   true,
		Messages: messages,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return err
	}

	response, err := http.Post(
		"http://localhost:11434/api/chat",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		var data map[string]interface{}

		if err := json.Unmarshal(scanner.Bytes(), &data); err != nil {
			return err
		}

		if message, ok := data["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				callback(content)
			}
		}
	}

	return scanner.Err()
}

func main() {
	app := tview.NewApplication()

	chatsMutex.Lock()
	chats, err := loadChats()
	if err != nil {
		fmt.Printf("Error to load chats %v:", err)
		chats = make(map[string]*ChatSession)
	}
	chatsMutex.Unlock()

	mainFlex := tview.NewFlex()

	chatList := tview.NewList().
		ShowSecondaryText(false).
		AddItem("New Chat", "", 'n', nil)
	chatList.SetBorder(true).SetTitle("Chats")

	var chatOrder []*ChatSession
	chatsMutex.RLock()
	for _, chat := range chats {
		chatOrder = append(chatOrder, chat)
	}
	chatsMutex.RUnlock()

	sort.Slice(chatOrder, func(i, j int) bool {
		return chatOrder[i].CreatedAt.Before(chatOrder[j].CreatedAt)
	})

	for _, chat := range chatOrder {
		chatList.AddItem(chat.Title, chat.ID.String(), 0, nil)
	}

	chatView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true)
	chatView.SetBorder(true).SetTitle("Chat")

	inputField := tview.NewInputField().
		SetLabel("You: ").
		SetFieldWidth(0)

	chatFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(chatView, 0, 1, false).
		AddItem(inputField, 1, 1, true)

	mainFlex.AddItem(chatList, 20, 1, false).
		AddItem(chatFlex, 0, 1, true)

	var currentChat *ChatSession

	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			userInput := inputField.GetText()
			inputField.SetText("")

			if currentChat == nil {
				currentChat = &ChatSession{
					ID:        uuid.New(),
					Title:     fmt.Sprintf("Chat %d", len(chats)+1),
					Messages:  []Message{},
					CreatedAt: time.Now(),
				}

				chatsMutex.Lock()
				chats[currentChat.ID.String()] = currentChat
				chatList.AddItem(currentChat.Title, currentChat.ID.String(), 0, nil)
				chatsMutex.Unlock()
			}

			currentChat.Messages = append(currentChat.Messages, Message{
				Role:    "user",
				Content: userInput,
			})

			currentChat.Messages = append(currentChat.Messages, Message{
				Role:    "assistant",
				Content: "",
			})
			updateChatDisplay(chatView, currentChat)

			history := make([]Message, len(currentChat.Messages))
			copy(history, currentChat.Messages)

			go func() {
				assistantIndex := len(history) - 1

				err := queryOllamaStream(history[:len(history)-1], func(chunck string) {
					app.QueueUpdateDraw(func() {
						if len(currentChat.Messages) > assistantIndex {
							currentChat.Messages[assistantIndex].Content += chunck

							updateChatDisplay(chatView, currentChat)
						}
					})
				})

				if err != nil {
					app.QueueUpdateDraw(func() {
						currentChat.Messages[assistantIndex].Content += "\n\n[red]" + "Error: " + err.Error()
					})
				}

				chatsMutex.Lock()
				defer chatsMutex.Unlock()
				if err := saveChat(currentChat); err != nil {
					app.QueueUpdateDraw(func() {
						currentChat.Messages[assistantIndex].Content += "\n\n[red]Error to save " + err.Error() + "[-]"
						updateChatDisplay(chatView, currentChat)
					})
				}
			}()
		}
	})

	chatList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyDelete || event.Rune() == 'd' {
			currentItem := chatList.GetCurrentItem()
			if currentItem > 0 {
				_, secondary := chatList.GetItemText(currentItem)

				modal := tview.NewModal().
					SetText("Delete this chat permanently?").
					AddButtons([]string{"Cancel", "Delete"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						if buttonLabel == "Delete" {
							chatsMutex.Lock()
							defer chatsMutex.Unlock()

							if chat, exists := chats[secondary]; exists {
								if err := deleteChat(chat, chats); err == nil {
									chatList.RemoveItem(currentItem)

									if currentChat != nil && currentChat.ID.String() == secondary {
										currentChat = nil
										chatView.Clear()
									}
								} else {
									app.QueueUpdateDraw(func() {
										chatView.SetText(fmt.Sprintf("[red]Error deleting chat: %v[-]", err))
									})
								}
							}
						}

						app.SetRoot(mainFlex, true)
					})

				app.SetRoot(modal, false)
			}

			return nil
		}

		return event
	})

	chatList.SetSelectedFunc(func(index int, title, secondary string, shortcut rune) {
		if index == 0 {
			currentChat = nil
			chatView.Clear()
			inputField.SetText("")
		} else {
			chatsMutex.RLock()
			defer chatsMutex.RUnlock()
			if chat, exists := chats[secondary]; exists {
				currentChat = chat
				updateChatDisplay(chatView, currentChat)
			}
		}
	})

	if err := app.SetRoot(mainFlex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
