package cli

import (
	"flag"
	"os"

	"github.com/charlesnpx/paperclip/internal/domain"
)

func conversationFlags(flags *flag.FlagSet, conversation *domain.Conversation) {
	flags.StringVar(&conversation.Harness, "harness", os.Getenv("PAPERCLIP_HARNESS"), "agent harness")
	flags.StringVar(&conversation.SessionID, "session-id", os.Getenv("PAPERCLIP_SESSION_ID"), "conversation ID")
	flags.StringVar(&conversation.SessionPath, "session-path", os.Getenv("PAPERCLIP_SESSION_PATH"), "transcript path")
	flags.StringVar(&conversation.SessionURL, "session-url", os.Getenv("PAPERCLIP_SESSION_URL"), "conversation URL")
}
