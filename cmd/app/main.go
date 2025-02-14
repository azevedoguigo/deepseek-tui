package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/azevedoguigo/deepseek-tui/internal/storage"
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

type OllamaRequest struct {
	Model    string            `json:"model"`
	Prompt   string            `json:"prompt"`
	Stream   bool              `json:"stream"`
	Messages []storage.Message `json:"messages"`
}

func updateChatDisplay(chatView *tview.TextView, chat *storage.ChatSession) {
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

func queryOllamaStream(messages []storage.Message, callback func(string)) error {
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
	chats, err := storage.LoadChats(configDir, chatsDir)
	if err != nil {
		fmt.Printf("Error to load chats %v:", err)
		chats = make(map[string]*storage.ChatSession)
	}
	chatsMutex.Unlock()

	mainFlex := tview.NewFlex()

	chatList := tview.NewList().
		ShowSecondaryText(false).
		AddItem("New Chat", "", 'n', nil)
	chatList.SetBorder(true).SetTitle("Chats")

	var chatOrder []*storage.ChatSession
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

	var currentChat *storage.ChatSession

	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			userInput := inputField.GetText()
			inputField.SetText("")

			if currentChat == nil {
				currentChat = &storage.ChatSession{
					ID:        uuid.New(),
					Title:     fmt.Sprintf("Chat %d", len(chats)+1),
					Messages:  []storage.Message{},
					CreatedAt: time.Now(),
				}

				chatsMutex.Lock()
				chats[currentChat.ID.String()] = currentChat
				chatList.AddItem(currentChat.Title, currentChat.ID.String(), 0, nil)
				chatsMutex.Unlock()
			}

			currentChat.Messages = append(currentChat.Messages, storage.Message{
				Role:    "user",
				Content: userInput,
			})

			currentChat.Messages = append(currentChat.Messages, storage.Message{
				Role:    "assistant",
				Content: "",
			})
			updateChatDisplay(chatView, currentChat)

			history := make([]storage.Message, len(currentChat.Messages))
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
				if err := storage.SaveChat(configDir, chatsDir, currentChat); err != nil {
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
								if err := storage.DeleteChat(chat, chats); err == nil {
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
