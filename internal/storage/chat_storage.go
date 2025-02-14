package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
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

func EnsureConfigDir(configDir, chatsDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(home, configDir, chatsDir)
	return os.MkdirAll(path, 0755)
}

func SaveChat(configDir, chatsDir string, session *ChatSession) error {
	if err := EnsureConfigDir(configDir, chatsDir); err != nil {
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

func LoadChats(configDir, chatsDir string) (map[string]*ChatSession, error) {
	if err := EnsureConfigDir(chatsDir, chatsDir); err != nil {
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

func DeleteChat(chat *ChatSession, chats map[string]*ChatSession) error {
	if chat.FilePath == "" {
		return fmt.Errorf("chat path not found")
	}

	if err := os.Remove(chat.FilePath); err != nil {
		return err
	}

	delete(chats, chat.ID.String())

	return nil
}
