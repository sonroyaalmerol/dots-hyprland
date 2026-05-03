#!/bin/bash
# AI summary for selected text (requires a running ollama model)
# Gets the current primary selection and sends it to ollama for summarization

SELECTION=$(xclip -selection primary -o 2>/dev/null || wl-paste -p 2>/dev/null)

if [ -z "$SELECTION" ]; then
    notify-send "AI Summary" "No text selected" -a "Hyprland"
    exit 1
fi

# Send to ollama and show notification with the response
RESPONSE=$(echo "$SELECTION" | ollama run llama3.2 "Summarize the following text concisely:" 2>/dev/null)

if [ -z "$RESPONSE" ]; then
    notify-send "AI Summary" "Failed to get response from ollama" -a "Hyprland" -u critical
    exit 1
fi

# Copy response to clipboard and notify
echo "$RESPONSE" | wl-copy
notify-send "AI Summary" "$RESPONSE" -a "Hyprland" -t 10000
