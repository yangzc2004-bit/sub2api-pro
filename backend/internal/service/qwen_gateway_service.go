package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

var qwenPassthroughAllowedHeaders = map[string]bool{
	"accept-language": true,
	"user-agent":      true,
}

var qwenMidtokenPattern = regexp.MustCompile(`(?:umx\.wu|__fycb)\('([^']+)'\)`)

const (
	qwenMaxAttachmentBytes     = 25 * 1024 * 1024
	qwenAttachmentParseTimeout = 60 * time.Second
	qwenAttachmentPollInterval = time.Second
	qwenInlineAttachmentLimit  = 120_000
	qwenInlineAttachmentEach   = 80_000
)

// ForwardQwenChatCompletions 将 OpenAI-compatible chat completions 请求
// 转发到 Qwen 网页版内部 API（chat.qwen.ai），使用 Session Token + Cookie 认证。
// 现已支持工具调用（Tool Calling）：将 tools 定义注入 prompt，解析模型输出的
// ##TOOL_CALL## 标记并转换为 OpenAI tool_calls 格式。
func (s *GatewayService) ForwardQwenChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeGatewayCCError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()
	reasoningEffort := extractCCReasoningEffortFromBody(body)

	upstreamModel := account.GetMappedModel(originalModel)
	upstreamModel = normalizeQwenWebModel(upstreamModel)

	// 解析请求体：提取 messages、tools、system prompt
	parsedReq, err := parseQwenRequestBody(body)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}

	// 构建带工具指令的 prompt
	prompt, err := buildQwenToolPrompt(&qwenToolPromptInput{
		SystemPrompt: parsedReq.System,
		Messages:     parsedReq.Messages,
		Tools:        parsedReq.Tools,
	})
	if err != nil {
		writeGatewayCCError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}

	authToken := account.GetQwenAuthToken()
	if authToken == "" {
		return nil, fmt.Errorf("account %d missing auth_token", account.ID)
	}
	cookie := account.GetQwenCookie()

	baseURL, err := s.validateUpstreamBaseURL(account.GetQwenBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid qwen base_url: %w", err)
	}

	midtoken, err := s.fetchQwenMidtoken(ctx, c, account)
	if err != nil {
		return nil, fmt.Errorf("fetch qwen bx-umidtoken: %w", err)
	}
	attachments, err := s.collectQwenAttachments(ctx, c, account, parsedReq.Messages)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	prompt = qwenPromptWithInlineAttachmentTexts(ctx, prompt, attachments)
	upstreamFiles, err := s.uploadQwenAttachments(ctx, c, account, baseURL, authToken, cookie, midtoken, attachments)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	chatID, chatCookies, err := s.createQwenChat(ctx, c, account, baseURL, authToken, cookie, midtoken, upstreamModel)
	if err != nil {
		return nil, err
	}
	upstreamBody, err := buildQwenChatCompletionsBody(chatID, upstreamModel, prompt, true, upstreamFiles)
	if err != nil {
		return nil, fmt.Errorf("build qwen chat body: %w", err)
	}
	targetURL := buildQwenAPIURL(baseURL, "/v2/chat/completions") + "?chat_id=" + chatID

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, clientStream)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	copyQwenPassthroughHeaders(c, upstreamReq)
	setQwenWebHeaders(upstreamReq, authToken, mergeQwenCookies(cookie, chatCookies), midtoken, account.GetCredential("bx_ua"))
	upstreamReq.Header.Set("Accept", "text/event-stream")

	resp, err := s.doQwenUpstream(ctx, c, account, upstreamReq)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return s.qwenFailoverError(ctx, c, account, resp)
		}
		return nil, s.writeQwenOpenAIError(c, resp)
	}

	// 传递 tools 信息到下游处理，用于流式/非流式响应中的工具调用检测
	tools := parsedReq.Tools
	if clientStream {
		return s.streamQwenChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime, tools)
	}
	return s.bufferQwenChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime, tools)
}

func (s *GatewayService) doQwenUpstream(ctx context.Context, c *gin.Context, account *Account, req *http.Request) (*http.Response, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	return resp, nil
}

func (s *GatewayService) fetchQwenMidtoken(ctx context.Context, c *gin.Context, account *Account) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://sg-wum.alibaba.com/w/wu.json", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", qwenDefaultUserAgent())
	req.Header.Set("Accept", "*/*")

	resp, err := s.doQwenUpstream(ctx, c, account, req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("midtoken upstream status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	match := qwenMidtokenPattern.FindSubmatch(body)
	if len(match) < 2 {
		return "", fmt.Errorf("failed to extract bx-umidtoken")
	}
	return string(match[1]), nil
}

func (s *GatewayService) createQwenChat(ctx context.Context, c *gin.Context, account *Account, baseURL, authToken, cookie, midtoken, model string) (string, []*http.Cookie, error) {
	payload := map[string]any{
		"title":     "New Chat",
		"models":    []string{model},
		"chat_mode": "normal",
		"chat_type": "t2t",
		"timestamp": time.Now().UnixMilli(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildQwenAPIURL(baseURL, "/v2/chats/new"), bytes.NewReader(payloadBytes))
	if err != nil {
		return "", nil, err
	}
	setQwenWebHeaders(req, authToken, cookie, midtoken, account.GetCredential("bx_ua"))

	resp, err := s.doQwenUpstream(ctx, c, account, req)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			_, failoverErr := s.qwenFailoverError(ctx, c, account, resp)
			return "", nil, failoverErr
		}
		return "", nil, s.writeQwenOpenAIError(c, resp)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", nil, fmt.Errorf("read qwen chat create response: %w", err)
	}
	chatID := strings.TrimSpace(gjson.GetBytes(respBody, "data.id").String())
	if chatID == "" || !gjson.GetBytes(respBody, "success").Bool() {
		return "", nil, fmt.Errorf("failed to create qwen chat: %s", sanitizeUpstreamErrorMessage(string(respBody)))
	}
	return chatID, resp.Cookies(), nil
}

func (s *GatewayService) qwenFailoverError(ctx context.Context, c *gin.Context, account *Account, resp *http.Response) (*ForwardResult, error) {
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "failover",
		Message:            upstreamMsg,
	})
	if s.rateLimitService != nil {
		s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
	}
	return nil, &UpstreamFailoverError{
		StatusCode:             resp.StatusCode,
		ResponseBody:           respBody,
		RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
	}
}

func (s *GatewayService) writeQwenOpenAIError(c *gin.Context, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = http.StatusText(resp.StatusCode)
	}
	c.Header("Content-Type", "application/json")
	c.JSON(mapUpstreamStatusCode(resp.StatusCode), gin.H{
		"error": gin.H{
			"type":    "server_error",
			"message": upstreamMsg,
		},
	})
	return fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func (s *GatewayService) streamQwenChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	tools []apicompat.ChatTool,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	respBody := resp.Body
	peekedBody, err := peekQwenStreamBody(resp.Body)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Failed to read upstream response")
		return nil, err
	}
	if peekedBody.upstreamError != "" {
		message := sanitizeUpstreamErrorMessage(peekedBody.upstreamError)
		if message == "" {
			message = "Qwen upstream request failed"
		}
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", message)
		return nil, fmt.Errorf("qwen upstream error: %s", message)
	}
	if len(peekedBody.prefix) > 0 {
		respBody = io.NopCloser(io.MultiReader(bytes.NewReader(peekedBody.prefix), resp.Body))
		defer func() { _ = respBody.Close() }()
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	// 当存在 tools 时，采用缓冲模式检测工具调用（qwen2API hybrid 模式思路）。
	// 这样可避免流式中途吐出不完整的 ##TOOL_CALL## 被客户端解析失败。
	hasTools := len(tools) > 0
	var textBuffer strings.Builder
	var usage ClaudeUsage
	var firstTokenMs *int
	clientDisconnected := false

	scanner := bufio.NewScanner(respBody)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			trimmedPayload := strings.TrimSpace(payload)
			if trimmedPayload != "" && trimmedPayload != "[DONE]" {
				if hasTools {
					// 缓冲文本内容，用于后续工具调用检测
					if text := qwenContentFromPayload(trimmedPayload); text != "" {
						textBuffer.WriteString(text)
					}
					if u := qwenUsageFromPayload(trimmedPayload); u != nil {
						usage = *u
					}
					if firstTokenMs == nil && textBuffer.Len() > 0 {
						elapsed := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &elapsed
					}
					continue // 不立即输出，等待完整响应后统一处理
				}

				chunks := qwenSSEPayloadToOpenAIChunks(payload, originalModel)
				if u := qwenUsageFromPayload(payload); u != nil {
					usage = *u
				}
				if firstTokenMs == nil && len(chunks) > 0 {
					elapsed := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &elapsed
				}
			}
		}
		outLines := []string{line + "\n"}
		if !hasTools {
			if payload, ok := extractOpenAISSEDataLine(line); ok {
				outLines = qwenSSEPayloadToOpenAISSELines(payload, originalModel)
			}
		}
		if !clientDisconnected {
			for _, outLine := range outLines {
				if _, err := c.Writer.WriteString(outLine); err != nil {
					clientDisconnected = true
					break
				}
			}
		}
		if line == "" && !clientDisconnected {
			c.Writer.Flush()
		}
	}
	if err := scanner.Err(); err != nil && !clientDisconnected && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("stream read error: %w", err)
	}

	// 如果启用了 tools，在流结束后检测工具调用并输出
	if hasTools && !clientDisconnected {
		if err := s.flushQwenStreamToolCalls(c, textBuffer.String(), originalModel, &usage, tools); err != nil {
			return nil, err
		}
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		ReasoningEffort:  reasoningEffort,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

// flushQwenStreamToolCalls 在流式响应结束时，检测工具调用并输出 SSE 事件。
func (s *GatewayService) flushQwenStreamToolCalls(
	c *gin.Context,
	fullText string,
	originalModel string,
	usage *ClaudeUsage,
	tools []apicompat.ChatTool,
) error {
	prefix, toolCalls, finishReason := parseQwenToolCalls(fullText)
	chunkID := "chatcmpl-qwen-" + uuid.NewString()
	created := time.Now().Unix()
	openaiToolCalls := qwenToolCallsToOpenAI(toolCalls, tools)

	if finishReason == "tool_calls" && len(openaiToolCalls) > 0 {
		// 先输出前缀文本（如果有）
		if prefix != "" {
			contentChunk := apicompat.ChatCompletionsChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{
					{
						Index: 0,
						Delta: apicompat.ChatDelta{
							Content: &prefix,
						},
					},
				},
			}
			if sse, err := apicompat.ChatChunkToSSE(contentChunk); err == nil {
				c.Writer.WriteString(sse)
			}
		}

		// 输出 tool_calls
		toolChunk := apicompat.ChatCompletionsChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   originalModel,
			Choices: []apicompat.ChatChunkChoice{
				{
					Index:        0,
					Delta:        apicompat.ChatDelta{ToolCalls: openaiToolCalls},
					FinishReason: &finishReason,
				},
			},
		}
		if sse, err := apicompat.ChatChunkToSSE(toolChunk); err == nil {
			c.Writer.WriteString(sse)
		}
	} else {
		// 无工具调用，作为普通文本输出
		if fullText != "" {
			contentChunk := apicompat.ChatCompletionsChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{
					{
						Index: 0,
						Delta: apicompat.ChatDelta{
							Content: &fullText,
						},
						FinishReason: qwenFallbackFinishReason(finishReason),
					},
				},
			}
			if sse, err := apicompat.ChatChunkToSSE(contentChunk); err == nil {
				c.Writer.WriteString(sse)
			}
		}
	}

	// 输出 usage（如果有）
	if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
		usageChunk := apicompat.ChatCompletionsChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   originalModel,
			Choices: []apicompat.ChatChunkChoice{},
			Usage: &apicompat.ChatUsage{
				PromptTokens:     usage.InputTokens,
				CompletionTokens: usage.OutputTokens,
				TotalTokens:      usage.InputTokens + usage.OutputTokens,
			},
		}
		if sse, err := apicompat.ChatChunkToSSE(usageChunk); err == nil {
			c.Writer.WriteString(sse)
		}
	}

	c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
	return nil
}

type qwenStreamBodyPeek struct {
	prefix        []byte
	upstreamError string
}

func peekQwenStreamBody(body io.Reader) (qwenStreamBodyPeek, error) {
	var result qwenStreamBodyPeek
	if body == nil {
		return result, fmt.Errorf("upstream response body is nil")
	}
	reader := bufio.NewReader(body)
	prefix, err := reader.Peek(1)
	if err != nil {
		if err == io.EOF {
			return result, fmt.Errorf("qwen upstream stream ended before data")
		}
		return result, fmt.Errorf("peek qwen upstream stream: %w", err)
	}
	if len(prefix) == 0 {
		return result, nil
	}
	if prefix[0] != '{' && prefix[0] != '[' {
		buffered := reader.Buffered()
		if buffered > 0 {
			result.prefix = make([]byte, buffered)
			if _, err := io.ReadFull(reader, result.prefix); err != nil {
				return result, fmt.Errorf("read qwen stream prefix: %w", err)
			}
		}
		return result, nil
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(reader, 2<<20))
	if err != nil {
		return result, fmt.Errorf("read qwen upstream json body: %w", err)
	}
	result.prefix = bodyBytes
	if gjson.ValidBytes(bodyBytes) {
		if msg := strings.TrimSpace(extractUpstreamErrorMessage(bodyBytes)); msg != "" {
			result.upstreamError = msg
			return result, nil
		}
		if success := gjson.GetBytes(bodyBytes, "success"); success.Exists() && !success.Bool() {
			result.upstreamError = strings.TrimSpace(gjson.GetBytes(bodyBytes, "data.details").String())
			if result.upstreamError == "" {
				result.upstreamError = strings.TrimSpace(gjson.GetBytes(bodyBytes, "data.code").String())
			}
			if result.upstreamError == "" {
				result.upstreamError = string(bodyBytes)
			}
		}
	}
	return result, nil
}

func (s *GatewayService) bufferQwenChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	tools []apicompat.ChatTool,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	ccResp, usage, err := readBufferedQwenChatCompletionsResponse(resp, tools)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Failed to read upstream response")
		return nil, err
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(c.Writer).Encode(ccResp); err != nil {
		return nil, fmt.Errorf("encode buffered qwen response: %w", err)
	}

	return &ForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

func copyQwenPassthroughHeaders(c *gin.Context, upstreamReq *http.Request) {
	if c == nil || c.Request == nil {
		return
	}
	for key, values := range c.Request.Header {
		if !qwenPassthroughAllowedHeaders[strings.ToLower(strings.TrimSpace(key))] {
			continue
		}
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
		}
	}
	upstreamReq.Header.Del("authorization")
	upstreamReq.Header.Del("x-api-key")
	upstreamReq.Header.Del("x-goog-api-key")
	upstreamReq.Header.Del("cookie")
}

func qwenDefaultUserAgent() string {
	return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"
}

func normalizeQwenWebModel(model string) string {
	switch normalizeQwenModelAlias(model) {
	case "qwen3.6-plus":
		return "qwen3.6-plus"
	case "qwen3.5-vl-plus":
		return "qwen3-vl-plus"
	default:
		return strings.TrimSpace(model)
	}
}

func setQwenWebHeaders(req *http.Request, authToken, cookie, midtoken, bxUA string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Source", "web")
	req.Header.Set("bx-umidtoken", midtoken)
	req.Header.Set("bx-v", "2.5.36")
	req.Header.Set("Version", "0.2.46")
	req.Header.Set("X-Accel-Buffering", "no")
	req.Header.Set("X-Request-Id", uuid.NewString())
	req.Header.Set("Timezone", time.Now().Format("Mon Jan 02 2006 15:04:05 GMT-0700"))
	req.Header.Set("User-Agent", qwenDefaultUserAgent())
	req.Header.Set("Origin", "https://chat.qwen.ai")
	req.Header.Set("Referer", "https://chat.qwen.ai/")
	if strings.TrimSpace(bxUA) != "" {
		req.Header.Set("bx-ua", strings.TrimSpace(bxUA))
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func buildQwenAPIURL(baseURL, path string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(normalized, "/api") {
		return normalized + path
	}
	return normalized + "/api" + path
}

func mergeQwenCookies(raw string, cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies)+1)
	if strings.TrimSpace(raw) != "" {
		parts = append(parts, strings.TrimSpace(raw))
	}
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func qwenPromptFromOpenAIChatBody(body []byte) (string, error) {
	if !gjson.ValidBytes(body) {
		return "", fmt.Errorf("invalid json body")
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return "", fmt.Errorf("messages is required")
	}
	var lastUser string
	sawUser := false
	var fallback string
	for _, msg := range messages.Array() {
		role := strings.ToLower(strings.TrimSpace(msg.Get("role").String()))
		text := qwenTextFromOpenAIContent(msg.Get("content"))
		if role == "user" {
			sawUser = true
			if strings.TrimSpace(text) == "" {
				lastUser = ""
				continue
			}
			lastUser = text
			continue
		}
		if strings.TrimSpace(text) != "" {
			fallback = text
		}
	}
	if strings.TrimSpace(lastUser) != "" {
		return lastUser, nil
	}
	if sawUser {
		return "", fmt.Errorf("message content is required")
	}
	if strings.TrimSpace(fallback) == "" {
		return "", fmt.Errorf("message content is required")
	}
	return fallback, nil
}

func qwenTextFromOpenAIContent(content gjson.Result) string {
	if !content.Exists() {
		return ""
	}
	if content.Type == gjson.String {
		return content.String()
	}
	if content.IsArray() {
		return qwenJoinTextParts(content.Array())
	}
	return qwenTextFromOpenAIContentObject(content)
}

func qwenJoinTextParts(parts []gjson.Result) string {
	var builder strings.Builder
	for _, part := range parts {
		text := qwenTextFromOpenAIContentPart(part)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func qwenTextFromOpenAIContentPart(part gjson.Result) string {
	if !part.Exists() {
		return ""
	}
	if part.Type == gjson.String {
		return part.String()
	}
	if part.IsArray() {
		return qwenJoinTextParts(part.Array())
	}
	partType := strings.ToLower(strings.TrimSpace(part.Get("type").String()))
	if qwenIsImageContentPartType(partType) {
		if ref := qwenImageURLFromContentPart(part); ref != "" {
			return "[Image attached: " + qwenAttachmentLabel(ref) + "]"
		}
		return "[Image attached]"
	}
	if !qwenIsTextContentPartType(partType) {
		return ""
	}
	return qwenTextFromOpenAIContentObject(part)
}

func qwenIsTextContentPartType(partType string) bool {
	switch partType {
	case "", "text", "input_text", "output_text", "message":
		return true
	default:
		return false
	}
}

func qwenIsImageContentPartType(partType string) bool {
	switch partType {
	case "image_url", "input_image", "image":
		return true
	default:
		return false
	}
}

func qwenImageURLFromContentPart(part gjson.Result) string {
	for _, path := range []string{"image_url.url", "image_url", "url", "source.url", "source"} {
		value := strings.TrimSpace(part.Get(path).String())
		if value != "" {
			return value
		}
	}
	return ""
}

func qwenAttachmentLabel(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "inline image"
	}
	if strings.HasPrefix(ref, "data:") {
		if comma := strings.Index(ref, ","); comma > 0 {
			meta := strings.TrimPrefix(ref[:comma], "data:")
			if meta != "" {
				return meta
			}
		}
		return "inline image"
	}
	if parsed, err := url.Parse(ref); err == nil && parsed.Path != "" {
		name := strings.TrimSpace(path.Base(parsed.Path))
		if name != "" && name != "." && name != "/" {
			return name
		}
	}
	return ref
}

func qwenTextFromOpenAIContentObject(content gjson.Result) string {
	for _, path := range []string{
		"text",
		"input_text",
		"output_text",
		"content",
		"message.content",
		"data.text",
		"value",
	} {
		text := qwenTextFromOpenAIContent(content.Get(path))
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	for _, path := range []string{"parts", "input"} {
		nested := content.Get(path)
		if !nested.Exists() {
			continue
		}
		text := qwenTextFromOpenAIContent(nested)
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func buildQwenChatCompletionsBody(chatID, model, prompt string, stream bool, files ...[]map[string]any) ([]byte, error) {
	messageID := uuid.NewString()
	messageFiles := []map[string]any{}
	if len(files) > 0 && files[0] != nil {
		messageFiles = files[0]
	}
	if len(messageFiles) > 0 {
		prompt = qwenPromptWithFileInstruction(prompt, messageFiles)
	}
	payload := map[string]any{
		"stream":             stream,
		"incremental_output": stream,
		"version":            "2.1",
		"chat_id":            chatID,
		"chat_mode":          "normal",
		"model":              model,
		"parent_id":          nil,
		"messages": []map[string]any{
			{
				"fid":         messageID,
				"parentId":    nil,
				"childrenIds": []string{},
				"role":        "user",
				"content":     prompt,
				"user_action": "chat",
				"files":       messageFiles,
				"models":      []string{model},
				"chat_type":   "t2t",
				"feature_config": map[string]any{
					"thinking_enabled":     false,
					"output_schema":        "phase",
					"research_mode":        "normal",
					"function_calling":     false,
					"enable_tools":         false,
					"enable_function_call": false,
					"tool_choice":          "none",
					"auto_search":          false,
					"code_interpreter":     false,
					"plugins_enabled":      false,
				},
				"timestamp": time.Now().Unix(),
				"extra": map[string]any{
					"meta": map[string]any{
						"subChatType": "t2t",
					},
				},
				"sub_chat_type": "t2t",
				"parent_id":     nil,
			},
		},
	}
	return json.Marshal(payload)
}

func qwenPromptWithFileInstruction(prompt string, files []map[string]any) string {
	if len(files) == 0 {
		return prompt
	}
	names := make([]string, 0, len(files))
	for _, file := range files {
		name := strings.TrimSpace(qwenStringFromAny(file["name"]))
		if name == "" {
			name = strings.TrimSpace(qwenStringFromAny(file["filename"]))
		}
		if name == "" {
			name = strings.TrimSpace(qwenStringFromAny(file["id"]))
		}
		if name != "" {
			names = append(names, name)
		}
	}
	prefix := "Use the uploaded attachment(s) as context when answering."
	if len(names) > 0 {
		prefix = "Use the uploaded attachment(s) as context when answering: " + strings.Join(names, ", ") + "."
	}
	if strings.TrimSpace(prompt) == "" {
		return prefix
	}
	return prefix + "\n\n" + prompt
}

func qwenStringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func qwenPromptWithInlineAttachmentTexts(ctx context.Context, prompt string, attachments []qwenLocalAttachment) string {
	if len(attachments) == 0 {
		return prompt
	}
	var sections []string
	remaining := qwenInlineAttachmentLimit
	for _, attachment := range attachments {
		if remaining <= 0 {
			break
		}
		text := strings.TrimSpace(qwenExtractAttachmentText(ctx, attachment))
		if text == "" {
			continue
		}
		text = qwenLimitRunes(text, min(remaining, qwenInlineAttachmentEach))
		if strings.TrimSpace(text) == "" {
			continue
		}
		remaining -= len([]rune(text))
		sections = append(sections, fmt.Sprintf("### %s\n%s", attachment.Filename, text))
	}
	if len(sections) == 0 {
		return prompt
	}
	prefix := "Extracted text from user attachments follows. Answer from this content first, and do not claim that the attachment is unreadable.\n\n" + strings.Join(sections, "\n\n")
	if strings.TrimSpace(prompt) == "" {
		return prefix
	}
	return prefix + "\n\nUser question:\n" + prompt
}

func qwenExtractAttachmentText(ctx context.Context, attachment qwenLocalAttachment) string {
	contentType := strings.ToLower(strings.TrimSpace(attachment.ContentType))
	filename := strings.ToLower(strings.TrimSpace(attachment.Filename))
	switch {
	case strings.HasPrefix(contentType, "text/") || strings.HasSuffix(filename, ".txt") || strings.HasSuffix(filename, ".md") || strings.HasSuffix(filename, ".json") || strings.HasSuffix(filename, ".csv") || strings.HasSuffix(filename, ".log"):
		return qwenDecodeLikelyUTF8Text(attachment.Data)
	case contentType == "application/pdf" || strings.HasSuffix(filename, ".pdf"):
		if text := qwenExtractPDFTextWithPdftotext(ctx, attachment.Data); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func qwenDecodeLikelyUTF8Text(data []byte) string {
	text := strings.TrimPrefix(string(data), "\ufeff")
	return strings.TrimSpace(text)
}

func qwenExtractPDFTextWithPdftotext(ctx context.Context, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	tool := qwenFindPDFToText()
	if strings.TrimSpace(tool) == "" {
		return ""
	}
	tmp, err := os.CreateTemp("", "qwen-attachment-*.pdf")
	if err != nil {
		return ""
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return ""
	}
	if err := tmp.Close(); err != nil {
		return ""
	}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, tool, "-layout", "-enc", "UTF-8", tmpPath, "-")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return qwenDecodeLikelyUTF8Text(out)
}

func qwenFindPDFToText() string {
	if tool, err := exec.LookPath("pdftotext"); err == nil && strings.TrimSpace(tool) != "" {
		return tool
	}
	for _, candidate := range []string{
		`D:\texlive\2026\bin\windows\pdftotext.exe`,
		`D:\texlive\2025\bin\windows\pdftotext.exe`,
		`C:\Program Files\poppler\Library\bin\pdftotext.exe`,
		`C:\Program Files\poppler\bin\pdftotext.exe`,
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func qwenLimitRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "\n\n[content truncated]"
}

type qwenLocalAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type qwenFileParseStatus struct {
	Status string
	Detail string
}

type qwenSTSFileToken struct {
	FileID          string `json:"file_id"`
	FilePath        string `json:"file_path"`
	BucketName      string `json:"bucketname"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	SecurityToken   string `json:"security_token"`
}

func (s *GatewayService) prepareQwenUpstreamFiles(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL string,
	authToken string,
	cookie string,
	midtoken string,
	messages []apicompat.ChatMessage,
) ([]map[string]any, error) {
	attachments, err := s.collectQwenAttachments(ctx, c, account, messages)
	if err != nil {
		return nil, err
	}
	if len(attachments) == 0 {
		return nil, nil
	}
	return s.uploadQwenAttachments(ctx, c, account, baseURL, authToken, cookie, midtoken, attachments)
}

func (s *GatewayService) uploadQwenAttachments(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL string,
	authToken string,
	cookie string,
	midtoken string,
	attachments []qwenLocalAttachment,
) ([]map[string]any, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	files := make([]map[string]any, 0, len(attachments))
	for _, attachment := range attachments {
		remote, err := s.uploadQwenAttachment(ctx, c, account, baseURL, authToken, cookie, midtoken, attachment)
		if err != nil {
			if strings.TrimSpace(qwenExtractAttachmentText(ctx, attachment)) != "" {
				continue
			}
			return nil, err
		}
		files = append(files, remote)
	}
	return files, nil
}

func (s *GatewayService) collectQwenAttachments(ctx context.Context, c *gin.Context, account *Account, messages []apicompat.ChatMessage) ([]qwenLocalAttachment, error) {
	var attachments []qwenLocalAttachment
	for _, msg := range messages {
		if len(msg.Content) == 0 || msg.Role == "assistant" || msg.Role == "tool" {
			continue
		}
		for _, part := range qwenContentParts(msg.Content) {
			var err error
			attachments, err = s.collectQwenAttachmentsFromPart(ctx, c, account, attachments, part)
			if err != nil {
				return nil, err
			}
		}
	}
	return attachments, nil
}

func (s *GatewayService) collectQwenAttachmentsFromPart(ctx context.Context, c *gin.Context, account *Account, attachments []qwenLocalAttachment, part map[string]any) ([]qwenLocalAttachment, error) {
	partType := strings.ToLower(strings.TrimSpace(qwenStringFromMap(part, "type")))
	switch partType {
	case "image_url", "input_image", "image":
		ref := qwenImageURLFromMap(part)
		if ref != "" {
			attachment, err := s.qwenAttachmentFromImageRef(ctx, c, account, ref, len(attachments)+1)
			if err != nil {
				return nil, err
			}
			attachments = append(attachments, attachment)
		}
	case "input_file", "file", "document":
		attachment, ok, err := qwenInlineFileAttachment(part, len(attachments)+1)
		if err != nil {
			return nil, err
		}
		if ok {
			attachments = append(attachments, attachment)
		}
	}

	for _, key := range []string{"content", "parts", "input"} {
		for _, nested := range qwenNestedContentParts(part[key]) {
			var err error
			attachments, err = s.collectQwenAttachmentsFromPart(ctx, c, account, attachments, nested)
			if err != nil {
				return nil, err
			}
		}
	}
	return attachments, nil
}

func qwenNestedContentParts(value any) []map[string]any {
	switch v := value.(type) {
	case []any:
		parts := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if part, ok := item.(map[string]any); ok {
				parts = append(parts, part)
			}
		}
		return parts
	case []map[string]any:
		return v
	case map[string]any:
		return []map[string]any{v}
	default:
		return nil
	}
}

func qwenContentParts(content json.RawMessage) []map[string]any {
	var parts []map[string]any
	if err := json.Unmarshal(content, &parts); err == nil {
		return parts
	}
	var obj map[string]any
	if err := json.Unmarshal(content, &obj); err == nil {
		return []map[string]any{obj}
	}
	return nil
}

func qwenStringFromMap(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	value, ok := obj[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func qwenImageURLFromMap(part map[string]any) string {
	if value := strings.TrimSpace(qwenStringFromMap(part, "image_url")); value != "" {
		return value
	}
	if imageURL, ok := part["image_url"].(map[string]any); ok {
		if value := strings.TrimSpace(qwenStringFromMap(imageURL, "url")); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(qwenStringFromMap(part, "url")); value != "" {
		return value
	}
	if source, ok := part["source"].(map[string]any); ok {
		if value := strings.TrimSpace(qwenStringFromMap(source, "url")); value != "" {
			return value
		}
		if value := strings.TrimSpace(qwenStringFromMap(source, "data")); value != "" {
			mediaType := strings.TrimSpace(qwenStringFromMap(source, "media_type"))
			if mediaType == "" {
				mediaType = "image/png"
			}
			return "data:" + mediaType + ";base64," + value
		}
	}
	return ""
}

func (s *GatewayService) qwenAttachmentFromImageRef(ctx context.Context, c *gin.Context, account *Account, ref string, index int) (qwenLocalAttachment, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "data:") {
		contentType, raw, err := qwenDecodeDataURI(ref)
		if err != nil {
			return qwenLocalAttachment{}, err
		}
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
			return qwenLocalAttachment{}, fmt.Errorf("image_url data URI must be an image")
		}
		if len(raw) > qwenMaxAttachmentBytes {
			return qwenLocalAttachment{}, fmt.Errorf("qwen attachment exceeds %d bytes", qwenMaxAttachmentBytes)
		}
		return qwenLocalAttachment{
			Filename:    fmt.Sprintf("inline-image-%d%s", index, qwenExtensionForContentType(contentType, ".png")),
			ContentType: contentType,
			Data:        raw,
		}, nil
	}
	parsed, err := url.Parse(ref)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return qwenLocalAttachment{}, fmt.Errorf("unsupported qwen image_url; use http(s) URL or data URI")
	}
	return s.downloadQwenAttachment(ctx, c, account, ref, index)
}

func (s *GatewayService) downloadQwenAttachment(ctx context.Context, c *gin.Context, account *Account, ref string, index int) (qwenLocalAttachment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return qwenLocalAttachment{}, err
	}
	req.Header.Set("Accept", "image/*,application/octet-stream;q=0.8,*/*;q=0.5")
	req.Header.Set("User-Agent", qwenDefaultUserAgent())
	resp, err := s.doQwenUpstream(ctx, c, account, req)
	if err != nil {
		return qwenLocalAttachment{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return qwenLocalAttachment{}, fmt.Errorf("download qwen attachment failed: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, qwenMaxAttachmentBytes+1))
	if err != nil {
		return qwenLocalAttachment{}, fmt.Errorf("read qwen attachment: %w", err)
	}
	if len(raw) > qwenMaxAttachmentBytes {
		return qwenLocalAttachment{}, fmt.Errorf("qwen attachment exceeds %d bytes", qwenMaxAttachmentBytes)
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if semi := strings.Index(contentType, ";"); semi >= 0 {
		contentType = strings.TrimSpace(contentType[:semi])
	}
	if contentType == "" {
		contentType = http.DetectContentType(raw)
	}
	filename := qwenFilenameFromURL(ref)
	if filename == "" {
		filename = fmt.Sprintf("remote-image-%d%s", index, qwenExtensionForContentType(contentType, ".bin"))
	}
	return qwenLocalAttachment{Filename: filename, ContentType: contentType, Data: raw}, nil
}

func qwenInlineFileAttachment(part map[string]any, index int) (qwenLocalAttachment, bool, error) {
	part = qwenMergedNestedFilePart(part)
	filename := strings.TrimSpace(qwenStringFromMap(part, "filename"))
	if filename == "" {
		filename = strings.TrimSpace(qwenStringFromMap(part, "name"))
	}
	if filename == "" {
		filename = strings.TrimSpace(qwenStringFromMap(part, "title"))
	}
	if filename == "" {
		filename = fmt.Sprintf("attachment-%d.txt", index)
	}
	contentType := strings.TrimSpace(qwenStringFromMap(part, "mime_type"))
	if contentType == "" {
		contentType = strings.TrimSpace(qwenStringFromMap(part, "content_type"))
	}
	if contentType == "" {
		contentType = strings.TrimSpace(qwenStringFromMap(part, "media_type"))
	}
	if contentType == "" {
		contentType = "text/plain"
	}
	if source, ok := part["source"].(map[string]any); ok {
		if sourceType := strings.TrimSpace(qwenStringFromMap(source, "type")); sourceType == "base64" {
			sourceContentType := strings.TrimSpace(qwenStringFromMap(source, "media_type"))
			if sourceContentType == "" {
				sourceContentType = contentType
			}
			sourceFilename := filename
			if sourceFilename == fmt.Sprintf("attachment-%d.txt", index) {
				sourceFilename = fmt.Sprintf("attachment-%d%s", index, qwenExtensionForContentType(sourceContentType, ".bin"))
			}
			if sourceDataURI := strings.TrimSpace(qwenStringFromMap(source, "url")); strings.HasPrefix(sourceDataURI, "data:") {
				ct, raw, err := qwenDecodeDataURI(sourceDataURI)
				if err != nil {
					return qwenLocalAttachment{}, false, err
				}
				return qwenLocalAttachment{Filename: sourceFilename, ContentType: ct, Data: raw}, true, nil
			}
			value := strings.TrimSpace(qwenStringFromMap(source, "data"))
			if value != "" {
				raw, err := base64.StdEncoding.DecodeString(removeASCIIWhitespace(value))
				if err != nil {
					return qwenLocalAttachment{}, false, fmt.Errorf("invalid base64 attachment data: %w", err)
				}
				return qwenLocalAttachment{Filename: sourceFilename, ContentType: sourceContentType, Data: raw}, true, nil
			}
		}
	}
	for _, key := range []string{"file_data"} {
		value := strings.TrimSpace(qwenStringFromMap(part, key))
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "data:") {
			ct, raw, err := qwenDecodeDataURI(value)
			if err != nil {
				return qwenLocalAttachment{}, false, err
			}
			return qwenLocalAttachment{Filename: qwenFilenameWithDetectedExtension(filename, index, ct), ContentType: ct, Data: raw}, true, nil
		}
		raw, err := base64.StdEncoding.DecodeString(removeASCIIWhitespace(value))
		if err != nil {
			return qwenLocalAttachment{}, false, fmt.Errorf("invalid base64 attachment data: %w", err)
		}
		return qwenLocalAttachment{Filename: qwenFilenameWithDetectedExtension(filename, index, contentType), ContentType: contentType, Data: raw}, true, nil
	}
	for _, key := range []string{"text", "content"} {
		value := qwenStringFromMap(part, key)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "data:") {
			ct, raw, err := qwenDecodeDataURI(value)
			if err != nil {
				return qwenLocalAttachment{}, false, err
			}
			return qwenLocalAttachment{Filename: qwenFilenameWithDetectedExtension(filename, index, ct), ContentType: ct, Data: raw}, true, nil
		}
		return qwenLocalAttachment{Filename: filename, ContentType: contentType, Data: []byte(value)}, true, nil
	}
	for _, key := range []string{"data_base64", "data"} {
		value := strings.TrimSpace(qwenStringFromMap(part, key))
		if value == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(removeASCIIWhitespace(value))
		if err != nil {
			return qwenLocalAttachment{}, false, fmt.Errorf("invalid base64 attachment data: %w", err)
		}
		return qwenLocalAttachment{Filename: qwenFilenameWithDetectedExtension(filename, index, contentType), ContentType: contentType, Data: raw}, true, nil
	}
	return qwenLocalAttachment{}, false, nil
}

func qwenMergedNestedFilePart(part map[string]any) map[string]any {
	nested, ok := part["file"].(map[string]any)
	if !ok {
		return part
	}
	merged := make(map[string]any, len(part)+len(nested))
	for k, v := range nested {
		merged[k] = v
	}
	for k, v := range part {
		if k == "file" || k == "type" {
			continue
		}
		merged[k] = v
	}
	merged["type"] = qwenStringFromMap(part, "type")
	return merged
}

func qwenFilenameWithDetectedExtension(filename string, index int, contentType string) string {
	defaultName := fmt.Sprintf("attachment-%d.txt", index)
	if strings.TrimSpace(filename) != defaultName {
		return filename
	}
	return fmt.Sprintf("attachment-%d%s", index, qwenExtensionForContentType(contentType, ".bin"))
}

func qwenDecodeDataURI(dataURI string) (string, []byte, error) {
	comma := strings.Index(dataURI, ",")
	if !strings.HasPrefix(dataURI, "data:") || comma < 0 {
		return "", nil, fmt.Errorf("invalid data URI")
	}
	meta := strings.TrimSpace(strings.TrimPrefix(dataURI[:comma], "data:"))
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return "", nil, fmt.Errorf("data URI must be base64 encoded")
	}
	contentType := strings.TrimSpace(strings.Split(meta, ";")[0])
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	raw, err := base64.StdEncoding.DecodeString(removeASCIIWhitespace(dataURI[comma+1:]))
	if err != nil {
		return "", nil, fmt.Errorf("invalid data URI base64: %w", err)
	}
	return contentType, raw, nil
}

func removeASCIIWhitespace(value string) string {
	return strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "").Replace(value)
}

func qwenExtensionForContentType(contentType, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "application/pdf":
		return ".pdf"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "text/plain":
		return ".txt"
	}
	if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return fallback
}

func qwenFilenameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(path.Base(parsed.Path))
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

func (s *GatewayService) uploadQwenAttachment(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL string,
	authToken string,
	cookie string,
	midtoken string,
	attachment qwenLocalAttachment,
) (map[string]any, error) {
	if len(attachment.Data) == 0 {
		return nil, fmt.Errorf("qwen attachment %s is empty", attachment.Filename)
	}
	sts, err := s.fetchQwenFileSTSToken(ctx, c, account, baseURL, authToken, cookie, midtoken, attachment)
	if err != nil {
		return nil, err
	}
	if err := s.putQwenOSSObject(ctx, account, sts, attachment); err != nil {
		return nil, err
	}
	if qwenFileClassFromContentType(attachment.ContentType) == "image" {
		return qwenRemoteFileRef(sts, attachment, "success"), nil
	}
	parseStatus, err := s.parseQwenUploadedFile(ctx, c, account, baseURL, authToken, cookie, midtoken, sts.FileID)
	if err != nil {
		return nil, err
	}
	return qwenRemoteFileRef(sts, attachment, parseStatus), nil
}

func (s *GatewayService) fetchQwenFileSTSToken(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL string,
	authToken string,
	cookie string,
	midtoken string,
	attachment qwenLocalAttachment,
) (qwenSTSFileToken, error) {
	var result qwenSTSFileToken
	payloadBytes, err := json.Marshal(map[string]any{
		"filename": attachment.Filename,
		"filesize": len(attachment.Data),
		"filetype": "file",
	})
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildQwenAPIURL(baseURL, "/v2/files/getstsToken"), bytes.NewReader(payloadBytes))
	if err != nil {
		return result, err
	}
	setQwenWebHeaders(req, authToken, cookie, midtoken, account.GetCredential("bx_ua"))
	resp, err := s.doQwenUpstream(ctx, c, account, req)
	if err != nil {
		return result, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return result, err
	}
	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("qwen getstsToken failed: HTTP %d %s", resp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}
	if err := json.Unmarshal([]byte(gjson.GetBytes(body, "data").Raw), &result); err != nil {
		return result, fmt.Errorf("parse qwen getstsToken response: %w", err)
	}
	if result.FileID == "" || result.FilePath == "" || result.BucketName == "" || result.Endpoint == "" ||
		result.AccessKeyID == "" || result.AccessKeySecret == "" {
		return result, fmt.Errorf("qwen getstsToken response missing upload fields")
	}
	return result, nil
}

func (s *GatewayService) putQwenOSSObject(ctx context.Context, account *Account, sts qwenSTSFileToken, attachment qwenLocalAttachment) error {
	target, region, err := qwenOSSObjectTarget(sts)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(attachment.Data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", attachment.ContentType)
	req.Header.Set("Content-Length", strconv.Itoa(len(attachment.Data)))
	req.ContentLength = int64(len(attachment.Data))
	req.Header.Set("Content-MD5", qwenContentMD5(attachment.Data))
	req.Header.Set("x-oss-security-token", sts.SecurityToken)
	req.Header.Set("x-oss-content-sha256", "UNSIGNED-PAYLOAD")
	qwenSignOSSV4Request(req, sts, region)

	resp, err := s.doQwenOSSRequest(ctx, account, req)
	if err != nil {
		return fmt.Errorf("qwen OSS upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return fmt.Errorf("qwen OSS upload failed: HTTP %d %s", resp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}
	return nil
}

func qwenContentMD5(data []byte) string {
	sum := md5.Sum(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func qwenOSSObjectTarget(sts qwenSTSFileToken) (string, string, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(sts.Endpoint), "https://"), "http://")
	if endpoint == "" {
		return "", "", fmt.Errorf("qwen OSS endpoint is empty")
	}
	region := normalizeQwenOSSSignRegion(sts.Region)
	if region == "" {
		region = "cn-hangzhou"
	}
	objectKey := strings.TrimPrefix(sts.FilePath, "/")
	target := fmt.Sprintf("https://%s.%s/%s", sts.BucketName, endpoint, objectKey)
	return target, region, nil
}

func (s *GatewayService) verifyQwenOSSObject(ctx context.Context, account *Account, sts qwenSTSFileToken, attachment qwenLocalAttachment) error {
	target, region, err := qwenOSSObjectTarget(sts)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return err
	}
	if attachment.ContentType != "" {
		req.Header.Set("Content-Type", attachment.ContentType)
	}
	req.Header.Set("x-oss-security-token", sts.SecurityToken)
	req.Header.Set("x-oss-content-sha256", "UNSIGNED-PAYLOAD")
	qwenSignOSSV4Request(req, sts, region)

	resp, err := s.doQwenOSSRequest(ctx, account, req)
	if err != nil {
		return fmt.Errorf("qwen OSS verify failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return fmt.Errorf("qwen OSS verify failed: HTTP %d %s", resp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}
	got := resp.ContentLength
	if got < 0 {
		got, _ = strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Content-Length")), 10, 64)
	}
	want := int64(len(attachment.Data))
	if got == 0 && want > 0 {
		return fmt.Errorf("qwen OSS verify failed: uploaded object is empty, want %d bytes file_id=%s object=%s", want, sts.FileID, strings.TrimPrefix(sts.FilePath, "/"))
	}
	if got > 0 && got != want {
		return fmt.Errorf("qwen OSS verify failed: uploaded object size=%d, want %d file_id=%s object=%s", got, want, sts.FileID, strings.TrimPrefix(sts.FilePath, "/"))
	}
	return nil
}

func (s *GatewayService) doQwenOSSRequest(ctx context.Context, account *Account, req *http.Request) (*http.Response, error) {
	proxyURL := ""
	accountID := int64(0)
	concurrency := 0
	if account != nil {
		accountID = account.ID
		concurrency = account.Concurrency
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
	}
	if s.tlsFPProfileService != nil && account != nil {
		return s.httpUpstream.DoWithTLS(req.WithContext(ctx), proxyURL, accountID, concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	}
	return s.httpUpstream.DoWithTLS(req.WithContext(ctx), proxyURL, accountID, concurrency, nil)
}

func normalizeQwenOSSSignRegion(region string) string {
	region = strings.TrimSpace(region)
	if strings.HasPrefix(region, "oss-") {
		return strings.TrimPrefix(region, "oss-")
	}
	return region
}

func qwenSignOSSV4Request(req *http.Request, sts qwenSTSFileToken, region string) {
	now := time.Now().UTC()
	date := now.Format("20060102")
	ossDate := now.Format("20060102T150405Z")
	req.Header.Set("x-oss-date", ossDate)

	additionalHeaders := []string{"content-type"}
	if sts.SecurityToken != "" {
		additionalHeaders = append(additionalHeaders, "x-oss-security-token")
	}
	sort.Strings(additionalHeaders)
	req.Header.Set("x-oss-additional-headers", strings.Join(additionalHeaders, ";"))

	scope := strings.Join([]string{date, region, "oss", "aliyun_v4_request"}, "/")
	canonicalRequest := qwenOSSV4CanonicalRequest(req, additionalHeaders, sts.BucketName)
	hashedCanonical := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "OSS4-HMAC-SHA256\n" + ossDate + "\n" + scope + "\n" + hex.EncodeToString(hashedCanonical[:])
	signingKey := qwenOSSV4SigningKey(sts.AccessKeySecret, date, region)
	signature := hex.EncodeToString(qwenHMACSHA256(signingKey, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"OSS4-HMAC-SHA256 Credential=%s/%s,AdditionalHeaders=%s,Signature=%s",
		sts.AccessKeyID,
		scope,
		strings.Join(additionalHeaders, ";"),
		signature,
	))
}

func qwenOSSV4CanonicalRequest(req *http.Request, additionalHeaders []string, bucketName string) string {
	canonicalURI := qwenOSSV4CanonicalURI(req, bucketName)
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := qwenCanonicalQuery(req.URL.Query())
	canonicalHeaders := qwenCanonicalOSSHeaders(req)
	additional := strings.Join(additionalHeaders, ";")
	payloadHash := strings.TrimSpace(req.Header.Get("x-oss-content-sha256"))
	if payloadHash == "" {
		payloadHash = "UNSIGNED-PAYLOAD"
	}
	return strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		additional,
		payloadHash,
	}, "\n")
}

func qwenOSSV4CanonicalURI(req *http.Request, bucketName string) string {
	canonicalURI := "/"
	if req != nil && req.URL != nil {
		canonicalURI = req.URL.EscapedPath()
	}
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	bucketName = strings.Trim(strings.TrimSpace(bucketName), "/")
	if bucketName == "" {
		return canonicalURI
	}
	bucketPrefix := "/" + bucketName
	if canonicalURI == bucketPrefix || strings.HasPrefix(canonicalURI, bucketPrefix+"/") {
		return canonicalURI
	}
	if canonicalURI == "/" {
		return bucketPrefix + "/"
	}
	return bucketPrefix + canonicalURI
}

func qwenCanonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0)
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		encodedKey := url.QueryEscape(key)
		for _, value := range vals {
			parts = append(parts, encodedKey+"="+url.QueryEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func qwenCanonicalOSSHeaders(req *http.Request) string {
	headers := make(map[string]string)
	for key, values := range req.Header {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "" {
			continue
		}
		if lower == "content-md5" || lower == "content-type" || strings.HasPrefix(lower, "x-oss-") {
			trimmedValues := make([]string, 0, len(values))
			for _, value := range values {
				trimmedValues = append(trimmedValues, strings.Join(strings.Fields(value), " "))
			}
			headers[lower] = strings.Join(trimmedValues, ",")
		}
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte(':')
		builder.WriteString(headers[key])
		builder.WriteByte('\n')
	}
	return builder.String()
}

func qwenOSSV4SigningKey(secret, date, region string) []byte {
	kDate := qwenHMACSHA256([]byte("aliyun_v4"+secret), date)
	kRegion := qwenHMACSHA256(kDate, region)
	kService := qwenHMACSHA256(kRegion, "oss")
	return qwenHMACSHA256(kService, "aliyun_v4_request")
}

func qwenHMACSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func (s *GatewayService) parseQwenUploadedFile(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	baseURL string,
	authToken string,
	cookie string,
	midtoken string,
	fileID string,
) (string, error) {
	if err := s.postQwenFileParse(ctx, c, account, baseURL, authToken, cookie, midtoken, fileID); err != nil {
		return "", err
	}
	deadline := time.Now().Add(qwenAttachmentParseTimeout)
	for {
		parseStatus, err := s.fetchQwenFileParseStatus(ctx, c, account, baseURL, authToken, cookie, midtoken, fileID)
		if err != nil {
			return "", err
		}
		status := parseStatus.Status
		switch strings.ToLower(status) {
		case "success":
			return status, nil
		case "failed", "error":
			if parseStatus.Detail != "" {
				return "", fmt.Errorf("qwen file parse failed: %s %s", status, parseStatus.Detail)
			}
			return "", fmt.Errorf("qwen file parse failed: %s", status)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("qwen file parse timeout: %s", status)
		}
		timer := time.NewTimer(qwenAttachmentPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *GatewayService) postQwenFileParse(ctx context.Context, c *gin.Context, account *Account, baseURL, authToken, cookie, midtoken, fileID string) error {
	payloadBytes, _ := json.Marshal(map[string]any{"file_id": fileID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildQwenAPIURL(baseURL, "/v2/files/parse"), bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	setQwenWebHeaders(req, authToken, cookie, midtoken, account.GetCredential("bx_ua"))
	resp, err := s.doQwenUpstream(ctx, c, account, req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("qwen files/parse failed: HTTP %d %s", resp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}
	return nil
}

func (s *GatewayService) fetchQwenFileParseStatus(ctx context.Context, c *gin.Context, account *Account, baseURL, authToken, cookie, midtoken, fileID string) (qwenFileParseStatus, error) {
	payloadBytes, _ := json.Marshal(map[string]any{"file_id_list": []string{fileID}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildQwenAPIURL(baseURL, "/v2/files/parse/status"), bytes.NewReader(payloadBytes))
	if err != nil {
		return qwenFileParseStatus{}, err
	}
	setQwenWebHeaders(req, authToken, cookie, midtoken, account.GetCredential("bx_ua"))
	resp, err := s.doQwenUpstream(ctx, c, account, req)
	if err != nil {
		return qwenFileParseStatus{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return qwenFileParseStatus{}, err
	}
	if resp.StatusCode >= 400 {
		return qwenFileParseStatus{}, fmt.Errorf("qwen files/parse/status failed: HTTP %d %s", resp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}
	rows := gjson.GetBytes(body, "data").Array()
	if len(rows) == 0 {
		return qwenFileParseStatus{Status: "pending", Detail: sanitizeUpstreamErrorMessage(string(body))}, nil
	}
	row := rows[0]
	status := strings.TrimSpace(row.Get("status").String())
	if status == "" {
		status = "pending"
	}
	detail := strings.TrimSpace(row.Raw)
	if detail == "" {
		detail = sanitizeUpstreamErrorMessage(string(body))
	}
	return qwenFileParseStatus{Status: status, Detail: sanitizeUpstreamErrorMessage(detail)}, nil
}

func qwenRemoteFileRef(sts qwenSTSFileToken, attachment qwenLocalAttachment, parseStatus string) map[string]any {
	nowMs := time.Now().UnixMilli()
	filePath := strings.TrimPrefix(sts.FilePath, "/")
	userID := ""
	if slash := strings.Index(filePath, "/"); slash > 0 {
		userID = filePath[:slash]
	}
	endpoint := strings.TrimPrefix(strings.TrimPrefix(sts.Endpoint, "https://"), "http://")
	putURL := fmt.Sprintf("https://%s.%s/%s", sts.BucketName, endpoint, filePath)
	return map[string]any{
		"type": "file",
		"file": map[string]any{
			"created_at": nowMs,
			"data":       map[string]any{},
			"filename":   attachment.Filename,
			"hash":       nil,
			"id":         sts.FileID,
			"user_id":    userID,
			"meta": map[string]any{
				"name":         attachment.Filename,
				"size":         len(attachment.Data),
				"content_type": attachment.ContentType,
				"parse_meta": map[string]any{
					"parse_status": parseStatus,
				},
			},
			"update_at": nowMs,
		},
		"id":              sts.FileID,
		"url":             putURL,
		"name":            attachment.Filename,
		"collection_name": "",
		"progress":        0,
		"status":          "uploaded",
		"greenNet":        "success",
		"size":            len(attachment.Data),
		"error":           "",
		"itemId":          uuid.NewString(),
		"file_type":       attachment.ContentType,
		"showType":        "file",
		"file_class":      qwenFileClassFromContentType(attachment.ContentType),
		"uploadTaskId":    uuid.NewString(),
	}
}

func qwenFileClassFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	default:
		return "document"
	}
}

func qwenSSEPayloadToOpenAISSELines(payload, model string) []string {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return []string{"\n"}
	}
	if trimmed == "[DONE]" {
		return []string{"data: [DONE]\n\n"}
	}
	chunks := qwenSSEPayloadToOpenAIChunks(trimmed, model)
	lines := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		sse, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			continue
		}
		lines = append(lines, sse)
	}
	return lines
}

func qwenSSEPayloadToOpenAIChunks(payload, model string) []apicompat.ChatCompletionsChunk {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "[DONE]" {
		return nil
	}
	if gjson.Get(trimmed, "object").String() == "chat.completion.chunk" {
		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(trimmed), &chunk); err != nil {
			return nil
		}
		return []apicompat.ChatCompletionsChunk{chunk}
	}
	if !qwenShouldForwardPayload(trimmed) {
		return nil
	}

	content := qwenContentFromPayload(trimmed)
	finishReason := qwenFinishReasonFromPayload(trimmed)
	usage := qwenChatUsageFromPayload(trimmed)
	if content == "" && finishReason == nil && usage == nil {
		return nil
	}

	chunkModel := strings.TrimSpace(gjson.Get(trimmed, "model").String())
	if chunkModel == "" {
		chunkModel = model
	}
	if chunkModel == "" {
		chunkModel = "qwen"
	}
	chunkID := strings.TrimSpace(gjson.Get(trimmed, "id").String())
	if chunkID == "" {
		chunkID = strings.TrimSpace(gjson.Get(trimmed, "message_id").String())
	}
	if chunkID == "" {
		chunkID = "chatcmpl-qwen-" + uuid.NewString()
	}
	created := gjson.Get(trimmed, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}

	var chunks []apicompat.ChatCompletionsChunk
	if content != "" || finishReason != nil {
		delta := apicompat.ChatDelta{}
		if content != "" {
			text := content
			delta.Content = &text
		}
		chunks = append(chunks, apicompat.ChatCompletionsChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   chunkModel,
			Choices: []apicompat.ChatChunkChoice{
				{
					Index:        0,
					Delta:        delta,
					FinishReason: finishReason,
				},
			},
		})
	}
	if usage != nil {
		chunks = append(chunks, apicompat.ChatCompletionsChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   chunkModel,
			Choices: []apicompat.ChatChunkChoice{},
			Usage:   usage,
		})
	}
	return chunks
}

func qwenShouldForwardPayload(payload string) bool {
	eventType := strings.ToLower(strings.TrimSpace(firstExistingQwenString(payload,
		"event",
		"type",
		"data.event",
		"data.type",
	)))
	switch strings.ReplaceAll(eventType, "-", "_") {
	case "ping", "heartbeat", "keepalive", "keep_alive":
		return false
	}

	phase := strings.ToLower(strings.TrimSpace(firstExistingQwenString(payload,
		"choices.0.delta.phase",
		"choices.0.phase",
		"data.choices.0.delta.phase",
		"data.choices.0.phase",
		"phase",
		"data.phase",
	)))
	if phase == "" {
		return true
	}
	switch strings.ReplaceAll(phase, "-", "_") {
	case "thinking", "think", "thinking_summary", "reasoning", "reasoning_summary",
		"search", "searching", "web_search", "tool", "tool_call", "tool_calls", "plan":
		return false
	default:
		return true
	}
}

func qwenContentFromPayload(payload string) string {
	paths := []string{
		"choices.0.delta.content",
		"choices.0.delta.text",
		"choices.0.delta.answer",
		"choices.0.delta.extra.content",
		"choices.0.delta.extra.text",
		"choices.0.delta.extra.answer",
		"choices.0.delta.message.content",
		"choices.0.delta.message.extra.content",
		"choices.0.message.content",
		"choices.0.message.extra.content",
		"choices.0.content",
		"choices.0.text",
		"data.choices.0.delta.content",
		"data.choices.0.delta.text",
		"data.choices.0.delta.answer",
		"data.choices.0.delta.extra.content",
		"data.choices.0.delta.extra.text",
		"data.choices.0.delta.extra.answer",
		"data.choices.0.message.content",
		"data.choices.0.message.extra.content",
		"data.choices.0.content",
		"data.choices.0.text",
		"message.content",
		"message.extra.content",
		"data.message.content",
		"data.content",
		"data.answer",
		"data.response",
		"data.output.text",
		"output.text",
		"data.text",
		"response",
		"answer",
		"text",
		"content",
	}
	for _, path := range paths {
		result := gjson.Get(payload, path)
		if text := qwenTextFromPayloadResult(result); text != "" {
			return text
		}
	}
	return ""
}

func qwenTextFromPayloadResult(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	if result.Type == gjson.String {
		return result.String()
	}
	if result.IsArray() {
		var builder strings.Builder
		for _, item := range result.Array() {
			text := qwenTextFromPayloadResult(item)
			if text == "" {
				text = qwenTextFromPayloadResult(item.Get("text"))
			}
			if text == "" {
				text = qwenTextFromPayloadResult(item.Get("content"))
			}
			if text == "" {
				text = qwenTextFromPayloadResult(item.Get("value"))
			}
			if text == "" {
				continue
			}
			builder.WriteString(text)
		}
		return builder.String()
	}
	if result.Type == gjson.JSON {
		for _, path := range []string{"text", "content", "answer", "value"} {
			if text := qwenTextFromPayloadResult(result.Get(path)); text != "" {
				return text
			}
		}
	}
	return ""
}

func qwenFinishReasonFromPayload(payload string) *string {
	paths := []string{
		"choices.0.finish_reason",
		"choices.0.delta.finish_reason",
		"data.choices.0.finish_reason",
		"data.choices.0.delta.finish_reason",
		"data.finish_reason",
		"finish_reason",
	}
	for _, path := range paths {
		result := gjson.Get(payload, path)
		if result.Exists() && strings.TrimSpace(result.String()) != "" {
			value := strings.TrimSpace(result.String())
			return &value
		}
	}
	phase := strings.ToLower(strings.TrimSpace(firstExistingQwenString(payload,
		"choices.0.delta.phase",
		"choices.0.phase",
		"data.choices.0.delta.phase",
		"data.choices.0.phase",
		"phase",
		"data.phase",
	)))
	status := strings.ToLower(strings.TrimSpace(firstExistingQwenString(payload,
		"choices.0.delta.status",
		"choices.0.status",
		"data.choices.0.delta.status",
		"data.choices.0.status",
		"status",
		"data.status",
	)))
	phase = strings.ReplaceAll(phase, "-", "_")
	status = strings.ReplaceAll(status, "-", "_")
	if phase == "finished" || phase == "done" || phase == "answer_finished" ||
		status == "finished" || status == "done" || status == "completed" ||
		gjson.Get(payload, "done").Bool() || gjson.Get(payload, "data.done").Bool() {
		value := "stop"
		return &value
	}
	return nil
}

func qwenUsageFromPayload(payload string) *ClaudeUsage {
	chatUsage := qwenChatUsageFromPayload(payload)
	if chatUsage == nil {
		return nil
	}
	return &ClaudeUsage{
		InputTokens:          chatUsage.PromptTokens,
		OutputTokens:         chatUsage.CompletionTokens,
		CacheReadInputTokens: 0,
	}
}

func qwenChatUsageFromPayload(payload string) *apicompat.ChatUsage {
	promptTokens := int(firstExistingQwenInt(payload,
		"usage.prompt_tokens",
		"usage.input_tokens",
		"usage.inputTokens",
		"input_tokens",
	))
	completionTokens := int(firstExistingQwenInt(payload,
		"usage.completion_tokens",
		"usage.output_tokens",
		"usage.outputTokens",
		"output_tokens",
	))
	if promptTokens == 0 && completionTokens == 0 {
		return nil
	}
	totalTokens := int(firstExistingQwenInt(payload,
		"usage.total_tokens",
		"usage.totalTokens",
		"total_tokens",
	))
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	return &apicompat.ChatUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

func firstExistingQwenInt(payload string, paths ...string) int64 {
	for _, path := range paths {
		result := gjson.Get(payload, path)
		if result.Exists() {
			return result.Int()
		}
	}
	return 0
}

func firstExistingQwenString(payload string, paths ...string) string {
	for _, path := range paths {
		result := gjson.Get(payload, path)
		if result.Exists() {
			return result.String()
		}
	}
	return ""
}

func readBufferedQwenChatCompletionsResponse(resp *http.Response, tools []apicompat.ChatTool) (*apicompat.ChatCompletionsResponse, ClaudeUsage, error) {
	var usage ClaudeUsage
	if resp == nil || resp.Body == nil {
		return nil, usage, fmt.Errorf("upstream response body is nil")
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxLineSize)

	var (
		responseID       string
		responseObject   string
		responseModel    string
		responseCreated  int64
		finishReasonText string
		contentBuilder   strings.Builder
	)

	for scanner.Scan() {
		payload, ok := extractOpenAISSEDataLine(scanner.Text())
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(payload)
		if trimmed == "" {
			continue
		}
		if trimmed == "[DONE]" {
			break
		}

		chunks := qwenSSEPayloadToOpenAIChunks(trimmed, "")
		if len(chunks) == 0 {
			continue
		}

		if u := qwenUsageFromPayload(trimmed); u != nil {
			usage = *u
		}
		for _, chunk := range chunks {
			if chunk.ID != "" {
				responseID = chunk.ID
			}
			if chunk.Object != "" {
				responseObject = chunk.Object
			}
			if chunk.Model != "" {
				responseModel = chunk.Model
			}
			if chunk.Created != 0 {
				responseCreated = chunk.Created
			}
			if chunk.Usage != nil {
				usage.InputTokens = chunk.Usage.PromptTokens
				usage.OutputTokens = chunk.Usage.CompletionTokens
				if chunk.Usage.PromptTokensDetails != nil {
					usage.CacheReadInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
				}
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != nil && *choice.Delta.Content != "" {
					_, _ = contentBuilder.WriteString(*choice.Delta.Content)
				}
				if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
					finishReasonText = strings.TrimSpace(*choice.FinishReason)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, usage, fmt.Errorf("read qwen upstream stream: %w", err)
	}
	if contentBuilder.Len() == 0 && usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return nil, usage, fmt.Errorf("qwen upstream stream ended without content")
	}

	if responseID == "" {
		responseID = "chatcmpl-qwen-buffered"
	}
	if responseObject == "" {
		responseObject = "chat.completion"
	}
	if responseModel == "" {
		responseModel = "unknown"
	}
	if responseCreated == 0 {
		responseCreated = time.Now().Unix()
	}
	if finishReasonText == "" {
		finishReasonText = "stop"
	}

	content := contentBuilder.String()

	// 检测工具调用（仅当请求包含 tools 时）
	var message apicompat.ChatMessage
	message.Role = "assistant"
	if len(tools) > 0 {
		prefix, toolCalls, tcFinishReason := parseQwenToolCalls(content)
		openaiToolCalls := qwenToolCallsToOpenAI(toolCalls, tools)
		if tcFinishReason == "tool_calls" && len(openaiToolCalls) > 0 {
			// 构建带 tool_calls 的 assistant 消息
			if prefix != "" {
				message.Content = json.RawMessage(strconv.Quote(prefix))
			}
			message.ToolCalls = openaiToolCalls
			finishReasonText = "tool_calls"
		} else {
			message.Content = json.RawMessage(strconv.Quote(content))
		}
	} else {
		message.Content = json.RawMessage(strconv.Quote(content))
	}

	chatResp := &apicompat.ChatCompletionsResponse{
		ID:      responseID,
		Object:  responseObject,
		Created: responseCreated,
		Model:   responseModel,
		Choices: []apicompat.ChatChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: finishReasonText,
			},
		},
	}
	if usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CacheReadInputTokens > 0 {
		chatResp.Usage = &apicompat.ChatUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
		}
		if usage.CacheReadInputTokens > 0 {
			chatResp.Usage.PromptTokensDetails = &apicompat.ChatTokenDetails{
				CachedTokens: usage.CacheReadInputTokens,
			}
		}
	}

	return chatResp, usage, nil
}
