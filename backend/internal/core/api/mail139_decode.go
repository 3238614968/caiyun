package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// decode139MailResponse accepts JSON and the mailbox's single-quoted object
// format. It normalizes only string delimiters/literal tokens; it never evals
// provider text, guesses delivery success, or retries a send.
func decode139MailResponse(body string, target interface{}) error {
	if json.Valid([]byte(body)) {
		return json.Unmarshal([]byte(body), target)
	}
	var out strings.Builder
	for i := 0; i < len(body); {
		ch := body[i]
		if ch == '\'' || ch == '"' {
			quote := ch
			out.WriteByte('"')
			i++
			closed := false
			for i < len(body) {
				ch = body[i]
				i++
				if ch == quote {
					closed = true
					break
				}
				if ch == '\\' {
					if i == len(body) {
						return fmt.Errorf("邮箱响应字符串转义不完整")
					}
					next := body[i]
					i++
					if next == '\'' {
						out.WriteByte('\'')
					} else {
						out.WriteByte('\\')
						out.WriteByte(next)
					}
					continue
				}
				if ch == '"' {
					out.WriteByte('\\')
				}
				out.WriteByte(ch)
			}
			if !closed {
				return fmt.Errorf("邮箱响应字符串未闭合")
			}
			out.WriteByte('"')
			continue
		}
		if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' {
			start := i
			for i < len(body) && (body[i] >= 'A' && body[i] <= 'Z' || body[i] >= 'a' && body[i] <= 'z') {
				i++
			}
			word := body[start:i]
			switch word {
			case "None":
				word = "null"
			case "True":
				word = "true"
			case "False":
				word = "false"
			}
			out.WriteString(word)
			continue
		}
		out.WriteByte(ch)
		i++
	}
	return json.Unmarshal([]byte(out.String()), target)
}
