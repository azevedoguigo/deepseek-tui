# DeepSeek TUI

✨ An elegant terminal interface (TUI) for interacting with DeepSeek LLM models via Ollama, built in Go!


## Main Features![Gravação de tela de 2025-02-14 19-12-59](https://github.com/user-attachments/assets/83d0c418-4eff-4261-905f-c4846b05ad85)

🚀 **Intuitive Interface**
- Conversation list in the left sidebar
- Auto-scrolling main chat area
- Colored messages (user/assistant)
- Support multiple chat sessions

🤖 **AI integration**
- Connection to DeepSeek models via Ollama
- Real-time responses

⚙️ **Technology**
- Developed in Go with library [tview](https://github.com/rivo/tview)
- Responsive and adaptive design

## Prerequisites

- [Ollama](https://ollama.ai/) installed and running
- DeepSeek-R1 model installed (`ollama pull deepseek-r1`)
- Go 1.20+ for compilation

## Installation

```bash
git clone https://github.com/azevedoguigo/deepseek-tui
cd deepseek-tui
go mod download
cd cmd/app
go build -o deepseek-tui
./deepseek-tui
