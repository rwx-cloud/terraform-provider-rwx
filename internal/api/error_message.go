package api

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type ErrorMessage struct {
	Message    string       `json:"message"`
	StackTrace []StackEntry `json:"stack_trace,omitempty"`
	Frame      string       `json:"frame"`
	Advice     string       `json:"advice"`
}

type StackEntry struct {
	FileName string `json:"file_name"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Name     string `json:"name"`
}

// extractErrorMessage is a small helper function for parsing an API error message
func extractErrorMessage(reader io.Reader) string {
	errorStruct := struct {
		Error         string         `json:"error,omitempty"`
		Errors        []string       `json:"errors,omitempty"`
		ErrorMessages []ErrorMessage `json:"error_messages,omitempty"`
	}{}

	if err := json.NewDecoder(reader).Decode(&errorStruct); err != nil {
		return ""
	}

	if len(errorStruct.ErrorMessages) > 0 {
		var message strings.Builder
		for _, errorMessage := range errorStruct.ErrorMessages {
			message.WriteString("\n\n")
			message.WriteString(formatUserMessage(errorMessage.Message, errorMessage.Frame, errorMessage.StackTrace, errorMessage.Advice))
		}

		return message.String()
	}

	// Fallback to Error field
	if errorStruct.Error != "" {
		return errorStruct.Error
	}
	if len(errorStruct.Errors) > 0 {
		return strings.Join(errorStruct.Errors, "; ")
	}

	// Fallback to an empty string
	return ""
}

func formatUserMessage(message string, frame string, stackTrace []StackEntry, advice string) string {
	var builder strings.Builder

	if message != "" {
		builder.WriteString(message)
	}

	if frame != "" {
		builder.WriteString("\n")
		builder.WriteString(frame)
	}

	if len(stackTrace) > 0 {
		for i := len(stackTrace) - 1; i >= 0; i-- {
			stackEntry := stackTrace[i]
			builder.WriteString("\n")
			if stackEntry.Name != "" {
				builder.WriteString(fmt.Sprintf("  at %s (%s:%d:%d)", stackEntry.Name, stackEntry.FileName, stackEntry.Line, stackEntry.Column))
			} else {
				builder.WriteString(fmt.Sprintf("  at %s:%d:%d", stackEntry.FileName, stackEntry.Line, stackEntry.Column))
			}
		}
	}

	if advice != "" {
		builder.WriteString("\n")
		builder.WriteString(advice)
	}

	return builder.String()
}
