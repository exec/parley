# Parley Selfbot Assistant

Responds to @mentions on your behalf while you're offline, powered by Ollama cloud (`qwen3.5:cloud` by default).

## Setup

```bash
cd extras/selfbot
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

cp .env.example .env
# edit .env with your credentials
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `PARLEY_BASE_URL` | Yes | Your Parley instance URL |
| `PARLEY_TOKEN` | One of these | JWT token (grab from browser localStorage) |
| `PARLEY_EMAIL` + `PARLEY_PASSWORD` | One of these | Login credentials |
| `OLLAMA_KEY` | Yes | Ollama cloud API key |
| `OLLAMA_MODEL` | No | Model (default: `qwen3.5:cloud`) |
| `SELFBOT_ENABLED` | No | Set `true` to actually send replies (default: dry-run) |

## Running

```bash
# Dry run first — see what it would say without sending anything
SELFBOT_ENABLED=false python main.py

# When you're happy with its responses, enable it
SELFBOT_ENABLED=true python main.py
```

## Behavior

- Listens for `MESSAGE_CREATE` events across all servers/channels you're in
- Responds **only when @mentioned** (won't reply to every message)
- Keeps a rolling 10-message context window per channel for coherent conversation
- Rate-limits itself (one reply per 5 seconds per channel)
- Truncates responses at 1800 characters

## System prompt

The bot is instructed to:
- Identify itself as your assistant, not as you
- Be brief, warm, and helpful
- Never share personal info, make promises, or say anything inappropriate
- Politely decline sensitive topics

## As a process (systemd example)

```ini
[Unit]
Description=Parley Selfbot Assistant
After=network.target

[Service]
WorkingDirectory=/path/to/extras/selfbot
EnvironmentFile=/path/to/extras/selfbot/.env
ExecStart=/path/to/.venv/bin/python main.py
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```
