package handler

import (
	"io"
	"log/slog"
	"os"
)

// NewLogHandler escreve uma linha JSON por evento, no formato que a API e o agente de observabilidade
// esperam: timestamp, level, service, event (a mensagem) e os campos do evento. O CPF nunca entra no log.
func NewLogHandler(out io.Writer) slog.Handler {
	return NewServiceLogHandler(out, "customer-auth", os.Getenv("APP_ENVIRONMENT"), os.Getenv("APP_VERSION"))
}

// NewServiceLogHandler adiciona identidade de serviço, ambiente e revisão a todos os eventos JSON.
func NewServiceLogHandler(out io.Writer, service, environment, version string) slog.Handler {
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
	attrs := []slog.Attr{slog.String("service", service)}
	if environment != "" {
		attrs = append(attrs, slog.String("environment", environment))
	}
	if version != "" {
		attrs = append(attrs, slog.String("version", version))
	}
	return slog.NewJSONHandler(out, options).WithAttrs(attrs)
}
