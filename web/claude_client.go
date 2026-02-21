package web

import (
	"context"
	"fmt"
	"net/http"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func streamDiagnosis(ctx context.Context, apiKey, prompt string, w http.ResponseWriter, flusher http.Flusher) error {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	for stream.Next() {
		ev := stream.Current()
		switch event := ev.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch delta := event.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				chunk := delta.Text
				if chunk != "" {
					fmt.Fprintf(w, "data: {\"chunk\":%q}\n\n", chunk)
					flusher.Flush()
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return err
	}

	fmt.Fprintf(w, "data: {\"done\":true}\n\n")
	flusher.Flush()
	return nil
}
