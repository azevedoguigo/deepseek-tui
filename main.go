package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatSession struct {
	Title    string
	Messages []Message
}

type OllamaRequest struct {
	Model    string    `json:"model"`
	Prompt   string    `json:"prompt"`
	Stream   bool      `json:"stream"`
	Messages []Message `json:"messages"`
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

func queryOllamaSteam(messages []Message, callback func(string)) error {
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

	mainFlex := tview.NewFlex()

	chatList := tview.NewList().
		ShowSecondaryText(true).
		AddItem("New Chat", "", 'n', nil)
	chatList.SetBorder(true).SetTitle("Chats")

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
	chats := make(map[string]*ChatSession)

	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			userInput := inputField.GetText()
			inputField.SetText("")

			if currentChat == nil {
				currentChat = &ChatSession{
					Title:    fmt.Sprintf("Chat %d", len(chats)+1),
					Messages: []Message{},
				}
				chats[currentChat.Title] = currentChat
				chatList.AddItem(currentChat.Title, "", 0, nil)
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

				err := queryOllamaSteam(history[:len(history)-1], func(chunck string) {
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
			}()
		}
	})

	chatList.SetSelectedFunc(func(index int, title, secondary string, shortcut rune) {
		if index == 0 {
			currentChat = nil
			chatView.Clear()
			inputField.SetText("")
		} else if chat, exists := chats[title]; exists {
			currentChat = chat
			updateChatDisplay(chatView, chat)
		}
	})

	if err := app.SetRoot(mainFlex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
