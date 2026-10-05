package model

import (
	"claude2api/logger"
	"claude2api/utils"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ChatCompletionRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

// ToolCall is one OpenAI-style function call returned to the client, which
// then executes it and sends the result back as a role=tool message.
type ToolCall struct {
	Index    int      `json:"index"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function FuncCall `json:"function"`
}

type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAISrteamResponse 定义 OpenAI 的流式响应结构
type OpenAISrteamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// Choice 结构表示 OpenAI 返回的单个选项
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        Delta       `json:"delta"`
	Logprobs     interface{} `json:"logprobs"`
	FinishReason interface{} `json:"finish_reason"`
}

type NoStreamChoice struct {
	Index        int         `json:"index"`
	Message      Message     `json:"message"`
	Logprobs     interface{} `json:"logprobs"`
	FinishReason string      `json:"finish_reason"`
}

// Delta 结构用于存储返回的文本内容
type Delta struct {
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}
type Message struct {
	Role             string        `json:"role"`
	Content          string        `json:"content,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall    `json:"tool_calls,omitempty"`
	Refusal          interface{}   `json:"refusal"`
	Annotation       []interface{} `json:"annotation"`
}

type OpenAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []NoStreamChoice `json:"choices"`
	Usage   Usage            `json:"usage"`
}
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// requestedModel echoes the model id the client asked for back in the
// response: OpenAI clients expect the response model to match the request.
func requestedModel(gc *gin.Context) string {
	if v, ok := gc.Get("RequestedModel"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "claude-sonnet-5"
}

func ReturnOpenAIReasoningResponse(reasoning string, stream bool, gc *gin.Context) error {
	if stream {
		return streamReasoningResponse(reasoning, gc)
	}
	return nil
}

func streamReasoningResponse(reasoning string, gc *gin.Context) error {
	openAIResp := &OpenAISrteamResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   requestedModel(gc),
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: Delta{
					ReasoningContent: reasoning,
				},
				Logprobs:     nil,
				FinishReason: nil,
			},
		},
	}

	jsonBytes, err := json.Marshal(openAIResp)
	if err != nil {
		logger.Error(fmt.Sprintf("Error marshalling JSON: %v", err))
		return err
	}
	jsonBytes = append([]byte("data: "), jsonBytes...)
	jsonBytes = append(jsonBytes, []byte("\n\n")...)

	gc.Writer.Write(jsonBytes)
	gc.Writer.Flush()
	return nil
}

func ReturnOpenAIResponse(text string, stream bool, gc *gin.Context) error {
	if stream {
		return streamRespose(text, gc)
	} else {
		return noStreamResponse(text, gc)
	}
}

func streamRespose(text string, gc *gin.Context) error {
	openAIResp := &OpenAISrteamResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   requestedModel(gc),
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: Delta{
					Content: text,
				},
				Logprobs:     nil,
				FinishReason: nil,
			},
		},
	}

	jsonBytes, err := json.Marshal(openAIResp)
	jsonBytes = append([]byte("data: "), jsonBytes...)
	jsonBytes = append(jsonBytes, []byte("\n\n")...)
	if err != nil {
		logger.Error(fmt.Sprintf("Error marshalling JSON: %v", err))
		return err
	}

	// 发送数据
	gc.Writer.Write(jsonBytes)
	gc.Writer.Flush()
	return nil
}

// toolCallRe matches a fenced ```toolcall JSON block the model emits when it
// wants to invoke a client tool: ```toolcall {"name":"...","arguments":{...}}```
var toolCallRe = regexp.MustCompile("(?s)```toolcall\\s*(\\{.*?\\})\\s*```")

// ExtractToolCalls pulls toolcall blocks out of the model text. Returns the
// cleaned text plus OpenAI-style tool calls (nil when there are none).
func ExtractToolCalls(text string) (string, []ToolCall) {
	var calls []ToolCall
	cleaned := toolCallRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := toolCallRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		var raw struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(sub[1]), &raw); err != nil || raw.Name == "" {
			return ""
		}
		args := strings.TrimSpace(string(raw.Arguments))
		if args == "" {
			args = "{}"
		}
		calls = append(calls, ToolCall{
			Index: len(calls),
			ID:    "call_" + uuid.New().String()[:8],
			Type:  "function",
			Function: FuncCall{
				Name:      raw.Name,
				Arguments: args,
			},
		})
		return ""
	})
	return strings.TrimSpace(cleaned), calls
}

func ResponseWithTools(text string, reasoning string, stream bool, gc *gin.Context) error {
	cleaned, calls := ExtractToolCalls(text)
	if len(calls) == 0 {
		if stream {
			return streamRespose(text, gc)
		}
		return noStreamResponseWithReasoning(text, reasoning, gc)
	}
	if stream {
		return streamToolResponse(cleaned, calls, gc)
	}
	return noStreamToolResponseWithReasoning(cleaned, reasoning, calls, gc)
}

func noStreamToolResponseWithReasoning(text string, reasoning string, calls []ToolCall, gc *gin.Context) error {
	promptTokens := 0
	if v, ok := gc.Get("PromptTokens"); ok {
		promptTokens, _ = v.(int)
	}
	completionTokens := utils.EstimateCompletionTokens(text + reasoning)
	openAIResp := &OpenAIResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   requestedModel(gc),
		Choices: []NoStreamChoice{
			{
				Index: 0,
				Message: Message{
					Role:             "assistant",
					Content:          text,
					ReasoningContent: reasoning,
					ToolCalls:        calls,
				},
				Logprobs:     nil,
				FinishReason: "tool_calls",
			},
		},
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	// The dashboard reads these after the response is flushed.
	gc.Set("CompletionTokens", completionTokens)

	gc.JSON(200, openAIResp)
	return nil
}

func streamToolResponse(text string, calls []ToolCall, gc *gin.Context) error {
	openAIResp := &OpenAISrteamResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   requestedModel(gc),
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: Delta{
					Content:   text,
					ToolCalls: calls,
				},
				Logprobs:     nil,
				FinishReason: "tool_calls",
			},
		},
	}

	jsonBytes, err := json.Marshal(openAIResp)
	jsonBytes = append([]byte("data: "), jsonBytes...)
	jsonBytes = append(jsonBytes, []byte("\n\n")...)
	if err != nil {
		logger.Error(fmt.Sprintf("Error marshalling JSON: %v", err))
		return err
	}

	// 发送数据
	gc.Writer.Write(jsonBytes)
	gc.Writer.Flush()
	return nil
}

// StreamToolFilter lets a text stream pass through untouched, but holds back a
// leading ```toolcall block so it can be emitted as a tool_calls chunk at the
// end instead of leaking the raw JSON as content.
type StreamToolFilter struct {
	pending strings.Builder
	state   int // 0 = undecided, 1 = normal text, 2 = toolcall block
}

const toolCallFence = "```toolcall"

func (f *StreamToolFilter) Feed(text string, stream bool, gc *gin.Context) {
	if !stream || f.state == 1 {
		ReturnOpenAIResponse(text, stream, gc)
		return
	}
	f.pending.WriteString(text)
	s := strings.TrimLeft(f.pending.String(), " \t\r\n")
	if strings.HasPrefix(s, toolCallFence) {
		f.state = 2
		return
	}
	// Still a prefix of the fence — could become one, so wait.
	if len(s) < len(toolCallFence) && strings.HasPrefix(toolCallFence, s) {
		return
	}
	f.state = 1
	out := f.pending.String()
	f.pending.Reset()
	ReturnOpenAIResponse(out, stream, gc)
}

// Finish flushes buffered text: a toolcall block becomes a tool_calls chunk,
// anything else streams as content.
func (f *StreamToolFilter) Finish(gc *gin.Context) {
	if f.state != 2 {
		if f.pending.Len() > 0 {
			ReturnOpenAIResponse(f.pending.String(), true, gc)
		}
		return
	}
	_, calls := ExtractToolCalls(f.pending.String())
	if len(calls) == 0 {
		ReturnOpenAIResponse(f.pending.String(), true, gc)
		return
	}
	streamToolResponse("", calls, gc)
}

func noStreamResponse(text string, gc *gin.Context) error {
	return noStreamResponseWithReasoning(text, "", gc)
}

func noStreamResponseWithReasoning(text string, reasoning string, gc *gin.Context) error {
	promptTokens := 0
	if v, ok := gc.Get("PromptTokens"); ok {
		promptTokens, _ = v.(int)
	}
	completionTokens := utils.EstimateCompletionTokens(text + reasoning)
	openAIResp := &OpenAIResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   requestedModel(gc),
		Choices: []NoStreamChoice{
			{
				Index: 0,
				Message: Message{
					Role:             "assistant",
					Content:          text,
					ReasoningContent: reasoning,
				},
				Logprobs:     nil,
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	// The dashboard reads these after the response is flushed.
	gc.Set("CompletionTokens", completionTokens)

	gc.JSON(200, openAIResp)
	return nil
}
