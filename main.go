package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func queryOllama(prompt string, history []Message) (string, error) {
	requestData := OllamaRequest{
		Model:    "deepseek-r1",
		Prompt:   prompt,
		Stream:   false,
		Messages: history,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return "", err
	}

	response, err := http.Post(
		"http://localhost:11434/api/generate",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result["response"].(string), nil
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

			updateChatDisplay(chatView, currentChat)

			go func() {
				response, err := queryOllama(userInput, currentChat.Messages)
				if err != nil {
					app.QueueUpdateDraw(func() {
						currentChat.Messages = append(currentChat.Messages, Message{
							Role:    "assistant",
							Content: fmt.Sprintf("Error: %v", err),
						})
						updateChatDisplay(chatView, currentChat)
					})

					return
				}

				app.QueueUpdateDraw(func() {
					currentChat.Messages = append(currentChat.Messages, Message{
						Role:    "assistant",
						Content: response,
					})

					updateChatDisplay(chatView, currentChat)
				})
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
