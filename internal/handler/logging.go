package handler

import (
	"io"
	"log/slog"
)

// NewLogHandler escreve uma linha JSON por evento, no formato que a API e o agente de observabilidade
// esperam: timestamp, level, service, event (a mensagem) e os campos do evento. O CPF nunca entra no log.
func NewLogHandler(out io.Writer) slog.Handler {
	options := &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				attr.Key = "timestamp"
			case slog.MessageKey:
				attr.Key = "event"
			}
			return attr
		},
	}
	return slog.NewJSONHandler(out, options).WithAttrs([]slog.Attr{slog.String("service", "customer-auth")})
}
