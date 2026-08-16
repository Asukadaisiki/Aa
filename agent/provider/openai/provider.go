// Package openai implements DeepSeek's OpenAI-compatible Chat Completions API.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"a2aagent/agent/provider"
)

const defaultURL = "https://api.deepseek.com/chat/completions"

type Config struct {
	APIKey     string
	URL        string
	Model      string
	HTTPClient *http.Client
}

type Client struct {
	apiKey, url, model string
	httpClient         *http.Client
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("deepseek API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("deepseek model is required")
	}
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		url = defaultURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{apiKey: cfg.APIKey, url: url, model: cfg.Model, httpClient: httpClient}, nil
}

func NewClient(cfg Config) (*Client, error) { return New(cfg) }
func (c *Client) Name() string              { return "deepseek" }

type wireRequest struct {
	Model           string                   `json:"model"`
	Messages        []wireMessage            `json:"messages"`
	Tools           []wireTool               `json:"tools,omitempty"`
	MaxTokens       int                      `json:"max_tokens,omitempty"`
	Temperature     *float64                 `json:"temperature,omitempty"`
	Stop            []string                 `json:"stop,omitempty"`
	Thinking        *provider.Thinking       `json:"thinking,omitempty"`
	ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
	ResponseFormat  *provider.ResponseFormat `json:"response_format,omitempty"`
	Stream          bool                     `json:"stream,omitempty"`
}

type wireMessage struct {
	Role             provider.Role  `json:"role"`
	Content          any            `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	Name             string         `json:"name,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ToolCalls        []wireToolCall `json:"tool_calls,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function wireFunction `json:"function"`
}

type wireFunction struct{ Name, Arguments string }

func (w wireFunction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	}{w.Name, w.Arguments})
}

type wireTool struct {
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type wireToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

func (c *Client) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	if err := provider.ValidateRequest(req); err != nil {
		return provider.Response{}, err
	}
	payload, err := c.makeRequest(req, false)
	if err != nil {
		return provider.Response{}, err
	}
	body, err := c.do(ctx, payload)
	if err != nil {
		return provider.Response{}, err
	}
	var response wireResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return provider.Response{}, fmt.Errorf("decode openai response: %w", err)
	}
	return response.toProvider(), nil
}

func (c *Client) Stream(ctx context.Context, req provider.Request, handler provider.StreamHandler) (provider.Response, error) {
	if err := provider.ValidateRequest(req); err != nil {
		return provider.Response{}, err
	}
	if handler == nil {
		return provider.Response{}, fmt.Errorf("stream handler is required")
	}
	payload, err := c.makeRequest(req, true)
	if err != nil {
		return provider.Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return provider.Response{}, fmt.Errorf("create openai request: %w", err)
	}
	c.setHeaders(httpReq)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return provider.Response{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return provider.Response{}, readAPIError("openai", resp)
	}
	return readSSE(resp.Body, handler)
}

func (c *Client) makeRequest(req provider.Request, stream bool) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}
	if model == "" {
		return nil, fmt.Errorf("openai model is required")
	}
	w := wireRequest{Model: model, Messages: make([]wireMessage, 0, len(req.Messages)), MaxTokens: req.MaxTokens, Temperature: req.Temperature, Stop: req.Stop, Thinking: req.Thinking, ReasoningEffort: req.ReasoningEffort, ResponseFormat: req.ResponseFormat, Stream: stream}
	if req.System != "" {
		w.Messages = append(w.Messages, wireMessage{Role: provider.RoleSystem, Content: req.System})
	}
	for _, message := range req.Messages {
		wm := wireMessage{Role: message.Role, ReasoningContent: message.ReasoningContent, Name: message.Name, ToolCallID: message.ToolCallID}
		if message.Role == provider.RoleTool {
			wm.Content = message.Content
		}
		if message.Content != "" && message.Role != provider.RoleTool {
			wm.Content = message.Content
		}
		for _, call := range message.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{ID: call.ID, Type: "function", Function: wireFunction{Name: call.Name, Arguments: string(call.Arguments)}})
		}
		w.Messages = append(w.Messages, wm)
	}
	for _, tool := range req.Tools {
		w.Tools = append(w.Tools, wireTool{Type: "function", Function: wireToolFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters}})
	}
	return json.Marshal(w)
}

func (c *Client) do(ctx context.Context, payload []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create openai request: %w", err)
	}
	c.setHeaders(httpReq)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, readAPIError("openai", resp)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai response: %w", err)
	}
	return body, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
}

type wireResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string         `json:"content"`
			ReasoningContent string         `json:"reasoning_content"`
			ToolCalls        []wireToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (r wireResponse) toProvider() provider.Response {
	result := provider.Response{ID: r.ID, Model: r.Model, Usage: provider.Usage{InputTokens: r.Usage.PromptTokens, OutputTokens: r.Usage.CompletionTokens, TotalTokens: r.Usage.TotalTokens}}
	if len(r.Choices) == 0 {
		return result
	}
	choice := r.Choices[0]
	result.Content, result.ReasoningContent, result.StopReason = choice.Message.Content, choice.Message.ReasoningContent, choice.FinishReason
	for _, call := range choice.Message.ToolCalls {
		args := json.RawMessage(call.Function.Arguments)
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		result.ToolCalls = append(result.ToolCalls, provider.ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: args})
	}
	return result
}

type streamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func readSSE(body io.Reader, handler provider.StreamHandler) (provider.Response, error) {
	reader := bufio.NewReader(body)
	var result provider.Response
	var calls []provider.ToolCall
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return result, fmt.Errorf("read openai stream: %w", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				return result, fmt.Errorf("decode openai stream event: %w", err)
			}
			if result.ID == "" {
				result.ID, result.Model = chunk.ID, chunk.Model
			}
			if chunk.Usage.TotalTokens != 0 {
				result.Usage = provider.Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens, TotalTokens: chunk.Usage.TotalTokens}
			}
			for _, choice := range chunk.Choices {
				if choice.FinishReason != "" {
					result.StopReason = choice.FinishReason
				}
				if choice.Delta.ReasoningContent != "" {
					result.ReasoningContent += choice.Delta.ReasoningContent
					if err := handler(provider.StreamEvent{Type: provider.EventReasoningDelta, Reasoning: choice.Delta.ReasoningContent}); err != nil {
						return result, err
					}
				}
				if choice.Delta.Content != "" {
					result.Content += choice.Delta.Content
					if err := handler(provider.StreamEvent{Type: provider.EventTextDelta, Text: choice.Delta.Content}); err != nil {
						return result, err
					}
				}
				for _, delta := range choice.Delta.ToolCalls {
					for len(calls) <= delta.Index {
						calls = append(calls, provider.ToolCall{})
					}
					call := &calls[delta.Index]
					if delta.ID != "" {
						call.ID = delta.ID
					}
					if delta.Function.Name != "" {
						call.Name = delta.Function.Name
					}
					call.Arguments = append(call.Arguments, delta.Function.Arguments...)
					if err := handler(provider.StreamEvent{Type: provider.EventToolCallDelta, ToolCall: provider.ToolCallDelta{Index: delta.Index, ID: delta.ID, Name: delta.Function.Name, Arguments: delta.Function.Arguments}}); err != nil {
						return result, err
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
	}
	for i := range calls {
		if len(calls[i].Arguments) == 0 {
			calls[i].Arguments = json.RawMessage("{}")
		}
	}
	result.ToolCalls = calls
	if err := handler(provider.StreamEvent{Type: provider.EventDone, Usage: result.Usage, Response: &result}); err != nil {
		return result, err
	}
	return result, nil
}

func readAPIError(name string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	message := strings.TrimSpace(string(body))
	var decoded struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &decoded)
	if decoded.Error.Message != "" {
		message = decoded.Error.Message
	}
	return &provider.APIError{Provider: name, StatusCode: resp.StatusCode, RequestID: resp.Header.Get("x-request-id"), Message: message, Body: string(body)}
}
