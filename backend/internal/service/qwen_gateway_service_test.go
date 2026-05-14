package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/tidwall/gjson"
)

type qwenGatewayTestHTTPUpstream struct {
	responses []*http.Response
	requests  []*http.Request
}

func (u *qwenGatewayTestHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
}

func (u *qwenGatewayTestHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func qwenGatewayTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestReadBufferedQwenChatCompletionsResponse(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-qwen","object":"chat.completion.chunk","created":123,"model":"qwen3.6plus","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		``,
		`data: {"id":"chatcmpl-qwen","object":"chat.completion.chunk","created":123,"model":"qwen3.6plus","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(body)),
	}

	got, usage, err := readBufferedQwenChatCompletionsResponse(resp, nil)
	if err != nil {
		t.Fatalf("readBufferedQwenChatCompletionsResponse returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil response")
	}
	if got.Model != "qwen3.6plus" {
		t.Fatalf("expected model qwen3.6plus, got %q", got.Model)
	}
	if len(got.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(got.Choices))
	}
	if got.Choices[0].FinishReason != "stop" {
		t.Fatalf("expected finish reason stop, got %q", got.Choices[0].FinishReason)
	}

	var content string
	if err := contentFromChatMessage(got.Choices[0].Message, &content); err != nil {
		t.Fatalf("failed to decode assistant content: %v", err)
	}
	if content != "Hello there" {
		t.Fatalf("expected aggregated content %q, got %q", "Hello there", content)
	}

	if got.Usage == nil {
		t.Fatal("expected usage in buffered response")
	}
	if got.Usage.PromptTokens != 5 || got.Usage.CompletionTokens != 2 || got.Usage.TotalTokens != 7 {
		t.Fatalf("unexpected usage: %+v", got.Usage)
	}
	if usage.InputTokens != 5 || usage.OutputTokens != 2 {
		t.Fatalf("unexpected forward usage: %+v", usage)
	}
}

func TestReadBufferedQwenChatCompletionsResponseEmptyStream(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
	}

	got, _, err := readBufferedQwenChatCompletionsResponse(resp, nil)
	if err == nil {
		t.Fatal("expected error for empty qwen stream")
	}
	if got != nil {
		t.Fatalf("expected nil response, got %+v", got)
	}
}

func TestNormalizeQwenWebModel(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"qwen3.6-plus":    "qwen3.6-plus",
		"qwen3.6plus":     "qwen3.6-plus",
		"qwen3.6":         "qwen3.6",
		"qwen3.5-vl-plus": "qwen3-vl-plus",
	}
	for input, want := range tests {
		if got := normalizeQwenWebModel(input); got != want {
			t.Fatalf("normalizeQwenWebModel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestQwenModelAliasMapping(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformQwen,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"qwen3.6plus": "qwen3.6plus",
			},
		},
	}

	if !account.IsModelSupported("qwen3.6-plus") {
		t.Fatal("expected qwen3.6-plus to be supported by legacy qwen3.6plus mapping")
	}
	if got := account.GetMappedModel("qwen3.6-plus"); got != "qwen3.6-plus" {
		t.Fatalf("expected qwen3.6-plus mapping, got %q", got)
	}
	if got := account.GetMappedModel("qwen3.6plus"); got != "qwen3.6-plus" {
		t.Fatalf("expected qwen3.6plus to map to qwen3.6-plus, got %q", got)
	}
}

func TestBuildQwenChatCompletionsBodyUsesWebPayloadShape(t *testing.T) {
	t.Parallel()

	body, err := buildQwenChatCompletionsBody("chat-1", "qwen3.6-plus", "hi", true)
	if err != nil {
		t.Fatalf("buildQwenChatCompletionsBody returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode qwen body: %v", err)
	}
	if payload["stream"] != true || payload["incremental_output"] != true {
		t.Fatalf("expected streaming payload, got %+v", payload)
	}
	if payload["chat_id"] != "chat-1" || payload["model"] != "qwen3.6-plus" {
		t.Fatalf("unexpected root payload: %+v", payload)
	}
	if payload["version"] != "2.1" {
		t.Fatalf("qwen web payload must include version 2.1, got %+v", payload["version"])
	}
	if _, exists := payload["timestamp"]; exists {
		t.Fatal("qwen web payload must not include root timestamp")
	}

	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one message, got %+v", payload["messages"])
	}
	msg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("expected message object, got %+v", messages[0])
	}
	if msg["role"] != "user" || msg["content"] != "hi" {
		t.Fatalf("unexpected message: %+v", msg)
	}
	files, ok := msg["files"].([]any)
	if !ok || len(files) != 0 {
		t.Fatalf("expected empty files array, got %+v", msg["files"])
	}
	if _, exists := msg["timestamp"]; !exists {
		t.Fatal("qwen web message must include timestamp")
	}
	featureConfig, ok := msg["feature_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected feature_config object, got %+v", msg["feature_config"])
	}
	if featureConfig["thinking_enabled"] != false {
		t.Fatalf("expected thinking_enabled false, got %+v", featureConfig)
	}
	if featureConfig["output_schema"] != "phase" {
		t.Fatalf("expected output_schema phase, got %+v", featureConfig["output_schema"])
	}
	if featureConfig["research_mode"] != "normal" {
		t.Fatalf("expected research_mode normal, got %+v", featureConfig["research_mode"])
	}
	if featureConfig["function_calling"] != false || featureConfig["enable_tools"] != false ||
		featureConfig["enable_function_call"] != false || featureConfig["tool_choice"] != "none" {
		t.Fatalf("expected native tool calling disabled, got %+v", featureConfig)
	}
	if _, exists := featureConfig["thinking_budget"]; exists {
		t.Fatalf("qwen web feature_config must not include thinking_budget when thinking is disabled")
	}
	for _, key := range []string{"auto_thinking", "thinking_mode", "thinking_format"} {
		if _, exists := featureConfig[key]; exists {
			t.Fatalf("qwen web feature_config must not include %s", key)
		}
	}
}

func TestBuildQwenChatCompletionsBodyIncludesFiles(t *testing.T) {
	t.Parallel()

	body, err := buildQwenChatCompletionsBody("chat-1", "qwen3.6-plus", "describe", true, []map[string]any{
		{"id": "file-1", "name": "image.png", "file_class": "image"},
	})
	if err != nil {
		t.Fatalf("buildQwenChatCompletionsBody returned error: %v", err)
	}

	if got := gjson.GetBytes(body, "messages.0.files.0.id").String(); got != "file-1" {
		t.Fatalf("expected attached file id, got %q in %s", got, string(body))
	}
}

func TestQwenPromptWithInlineAttachmentTextsAddsTextAttachmentContent(t *testing.T) {
	t.Parallel()

	got := qwenPromptWithInlineAttachmentTexts(context.Background(), "summarize it", []qwenLocalAttachment{
		{Filename: "guide.txt", ContentType: "text/plain", Data: []byte("Install Docker\nRun docker compose up")},
	})

	if !strings.Contains(got, "guide.txt") || !strings.Contains(got, "Install Docker") || !strings.Contains(got, "User question") {
		t.Fatalf("expected inline attachment text in prompt, got %q", got)
	}
}

func TestQwenFindPDFToTextFindsAvailableExtractor(t *testing.T) {
	t.Parallel()

	if got := qwenFindPDFToText(); strings.TrimSpace(got) == "" {
		t.Fatal("expected pdftotext to be available for PDF attachment extraction")
	}
}

func TestUploadQwenAttachmentsSkipsUploadFailureWhenTextWasInlined(t *testing.T) {
	t.Parallel()

	upstream := &qwenGatewayTestHTTPUpstream{
		responses: []*http.Response{qwenGatewayTestResponse(http.StatusBadGateway, "bad gateway")},
	}
	svc := &GatewayService{httpUpstream: upstream}
	files, err := svc.uploadQwenAttachments(context.Background(), nil, &Account{}, "https://chat.qwen.ai", "token", "", "", []qwenLocalAttachment{
		{Filename: "guide.txt", ContentType: "text/plain", Data: []byte("already in prompt")},
	})
	if err != nil {
		t.Fatalf("uploadQwenAttachments should skip upload failure for inlined text attachment: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no uploaded files after skipped failure, got %+v", files)
	}
}

func TestQwenPromptFromOpenAIChatBodyUsesLatestUserMessage(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "qwen3.6-plus",
		"messages": [
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": "Hello! How can I help?"},
			{"role": "user", "content": "1+1等于几？"}
		]
	}`)

	got, err := qwenPromptFromOpenAIChatBody(body)
	if err != nil {
		t.Fatalf("qwenPromptFromOpenAIChatBody returned error: %v", err)
	}
	if got != "1+1等于几？" {
		t.Fatalf("expected latest user message, got %q", got)
	}
}

func TestQwenPromptFromOpenAIChatBodyAcceptsInputTextContentParts(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "qwen3.6-plus",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "1+1等于几？"}]},
			{"role": "assistant", "content": "1+1等于2。"},
			{"role": "user", "content": [{"type": "input_text", "text": "2+2呢"}]}
		]
	}`)

	got, err := qwenPromptFromOpenAIChatBody(body)
	if err != nil {
		t.Fatalf("qwenPromptFromOpenAIChatBody returned error: %v", err)
	}
	if got != "2+2呢" {
		t.Fatalf("expected latest input_text user message, got %q", got)
	}
}

func TestQwenPromptFromOpenAIChatBodyAcceptsNestedTextContent(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "qwen3.6-plus",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "one plus one"}]},
			{"role": "assistant", "content": "two"},
			{"role": "user", "content": [{"type": "message", "content": [{"type": "input_text", "text": "two plus two"}]}]}
		]
	}`)

	got, err := qwenPromptFromOpenAIChatBody(body)
	if err != nil {
		t.Fatalf("qwenPromptFromOpenAIChatBody returned error: %v", err)
	}
	if got != "two plus two" {
		t.Fatalf("expected nested latest user message, got %q", got)
	}
}

func TestQwenPromptFromOpenAIChatBodyKeepsImageOnlyLatestUser(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "qwen3.6-plus",
		"messages": [
			{"role": "user", "content": "old user"},
			{"role": "assistant", "content": "old answer"},
			{"role": "user", "content": [{"type": "image_url", "image_url": {"url": "https://example.com/a.png"}}]}
		]
	}`)

	got, err := qwenPromptFromOpenAIChatBody(body)
	if err != nil {
		t.Fatalf("qwenPromptFromOpenAIChatBody returned error: %v", err)
	}
	if got != "[Image attached: a.png]" {
		t.Fatalf("expected latest image placeholder, got %q", got)
	}
}

func TestQwenDecodeDataURI(t *testing.T) {
	t.Parallel()

	contentType, raw, err := qwenDecodeDataURI("data:image/png;base64,aGVsbG8=")
	if err != nil {
		t.Fatalf("qwenDecodeDataURI returned error: %v", err)
	}
	if contentType != "image/png" || string(raw) != "hello" {
		t.Fatalf("unexpected decoded data: %s %q", contentType, string(raw))
	}
}

func TestQwenExtensionForContentTypeUsesCommonUploadExtensions(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"image/jpeg":      ".jpg",
		"image/jpg":       ".jpg",
		"image/png":       ".png",
		"application/pdf": ".pdf",
		"text/plain":      ".txt",
	}
	for contentType, want := range tests {
		if got := qwenExtensionForContentType(contentType, ".bin"); got != want {
			t.Fatalf("qwenExtensionForContentType(%q) = %q, want %q", contentType, got, want)
		}
	}
}

func TestQwenInlineFileAttachmentSupportsAnthropicBase64Source(t *testing.T) {
	t.Parallel()

	attachment, ok, err := qwenInlineFileAttachment(map[string]any{
		"type": "file",
		"source": map[string]any{
			"type":       "base64",
			"media_type": "application/pdf",
			"data":       "JVBERi0xLjQK",
		},
	}, 1)
	if err != nil {
		t.Fatalf("qwenInlineFileAttachment returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected attachment")
	}
	if attachment.Filename != "attachment-1.pdf" {
		t.Fatalf("expected pdf filename, got %q", attachment.Filename)
	}
	if attachment.ContentType != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", attachment.ContentType)
	}
	if string(attachment.Data) != "%PDF-1.4\n" {
		t.Fatalf("unexpected data: %q", string(attachment.Data))
	}
}

func TestQwenInlineFileAttachmentSupportsOpenAINestedFileData(t *testing.T) {
	t.Parallel()

	attachment, ok, err := qwenInlineFileAttachment(map[string]any{
		"type": "file",
		"file": map[string]any{
			"filename":  "Sub2API-deploy.pdf",
			"file_data": "data:application/pdf;base64,JVBERi0xLjQK",
		},
	}, 1)
	if err != nil {
		t.Fatalf("qwenInlineFileAttachment returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected attachment")
	}
	if attachment.Filename != "Sub2API-deploy.pdf" {
		t.Fatalf("expected nested filename, got %q", attachment.Filename)
	}
	if attachment.ContentType != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", attachment.ContentType)
	}
	if string(attachment.Data) != "%PDF-1.4\n" {
		t.Fatalf("unexpected data: %q", string(attachment.Data))
	}
}

func TestQwenInlineFileAttachmentSupportsDocumentPart(t *testing.T) {
	t.Parallel()

	attachment, ok, err := qwenInlineFileAttachment(map[string]any{
		"type":  "document",
		"title": "Sub2API-deploy.pdf",
		"source": map[string]any{
			"type":       "base64",
			"media_type": "application/pdf",
			"data":       "JVBERi0xLjQK",
		},
	}, 1)
	if err != nil {
		t.Fatalf("qwenInlineFileAttachment returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected attachment")
	}
	if attachment.Filename != "Sub2API-deploy.pdf" {
		t.Fatalf("expected title filename, got %q", attachment.Filename)
	}
	if attachment.ContentType != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", attachment.ContentType)
	}
	if string(attachment.Data) != "%PDF-1.4\n" {
		t.Fatalf("unexpected data: %q", string(attachment.Data))
	}
}

func TestCollectQwenAttachmentsRecursesIntoNestedContentParts(t *testing.T) {
	t.Parallel()

	svc := &GatewayService{}
	messages := []apicompat.ChatMessage{
		{
			Role: "user",
			Content: json.RawMessage(`[
				{
					"type":"message",
					"content":[
						{"type":"text","text":"summarize"},
						{"type":"file","file":{"filename":"nested.pdf","file_data":"data:application/pdf;base64,JVBERi0xLjQK"}}
					]
				}
			]`),
		},
	}

	attachments, err := svc.collectQwenAttachments(context.Background(), nil, nil, messages)
	if err != nil {
		t.Fatalf("collectQwenAttachments returned error: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("expected one attachment, got %+v", attachments)
	}
	if attachments[0].Filename != "nested.pdf" || attachments[0].ContentType != "application/pdf" || string(attachments[0].Data) != "%PDF-1.4\n" {
		t.Fatalf("unexpected attachment: %+v", attachments[0])
	}
}

func TestQwenOSSV4CanonicalRequestMatchesAccelerateHostShape(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPut, "https://qwen-webui-prod.oss-accelerate.aliyuncs.com/user-1/object.jfif", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("x-oss-additional-headers", "content-type;x-oss-security-token")
	req.Header.Set("x-oss-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-oss-date", "20260510T043348Z")
	req.Header.Set("x-oss-security-token", "token")

	got := qwenOSSV4CanonicalRequest(req, []string{"content-type", "x-oss-security-token"}, "qwen-webui-prod")
	want := strings.Join([]string{
		"PUT",
		"/qwen-webui-prod/user-1/object.jfif",
		"",
		"content-type:image/jpeg\n" +
			"x-oss-additional-headers:content-type;x-oss-security-token\n" +
			"x-oss-content-sha256:UNSIGNED-PAYLOAD\n" +
			"x-oss-date:20260510T043348Z\n" +
			"x-oss-security-token:token\n",
		"content-type;x-oss-security-token",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	if got != want {
		t.Fatalf("unexpected canonical request:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if strings.Contains(got, "\nhost:") {
		t.Fatalf("canonical request must not include host when host is not in additional headers:\n%s", got)
	}
}

func TestQwenOSSV4CanonicalRequestIncludesContentMD5(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPut, "https://qwen-webui-prod.oss-accelerate.aliyuncs.com/user-1/object.jpg", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Content-MD5", "abc==")
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("x-oss-additional-headers", "content-type;x-oss-security-token")
	req.Header.Set("x-oss-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-oss-date", "20260510T132733Z")
	req.Header.Set("x-oss-security-token", "token")

	got := qwenOSSV4CanonicalRequest(req, []string{"content-type", "x-oss-security-token"}, "qwen-webui-prod")
	if !strings.Contains(got, "\ncontent-md5:abc==\ncontent-type:image/jpeg\n") {
		t.Fatalf("canonical request missing content-md5 before content-type:\n%s", got)
	}
}

func TestPutQwenOSSObjectSetsRequestBodyLength(t *testing.T) {
	t.Parallel()

	upstream := &qwenGatewayTestHTTPUpstream{
		responses: []*http.Response{qwenGatewayTestResponse(http.StatusOK, "")},
	}
	svc := &GatewayService{httpUpstream: upstream}
	attachment := qwenLocalAttachment{
		Filename:    "inline-image-1.jpg",
		ContentType: "image/jpeg",
		Data:        []byte("image-bytes"),
	}
	sts := qwenSTSFileToken{
		FileID:          "file-1",
		FilePath:        "user-1/object.jpg",
		BucketName:      "qwen-webui-prod",
		Endpoint:        "oss-accelerate.aliyuncs.com",
		Region:          "oss-ap-southeast-1",
		AccessKeyID:     "ak",
		AccessKeySecret: "secret",
		SecurityToken:   "token",
	}

	if err := svc.putQwenOSSObject(context.Background(), nil, sts, attachment); err != nil {
		t.Fatalf("putQwenOSSObject returned error: %v", err)
	}
	if len(upstream.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(upstream.requests))
	}
	req := upstream.requests[0]
	if req.ContentLength != int64(len(attachment.Data)) {
		t.Fatalf("ContentLength = %d, want %d", req.ContentLength, len(attachment.Data))
	}
	if got := req.Header.Get("Content-Length"); got != "11" {
		t.Fatalf("Content-Length header = %q, want 11", got)
	}
	if got := req.Header.Get("Content-MD5"); got == "" {
		t.Fatal("expected Content-MD5 header")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if string(body) != string(attachment.Data) {
		t.Fatalf("request body = %q, want %q", string(body), string(attachment.Data))
	}
}

func TestUploadQwenAttachmentSkipsParseForImages(t *testing.T) {
	t.Parallel()

	stsBody := `{"data":{"file_id":"file-1","file_path":"user-1/object.jpg","bucketname":"qwen-webui-prod","endpoint":"oss-accelerate.aliyuncs.com","region":"oss-ap-southeast-1","access_key_id":"ak","access_key_secret":"secret","security_token":"token"}}`
	upstream := &qwenGatewayTestHTTPUpstream{
		responses: []*http.Response{
			qwenGatewayTestResponse(http.StatusOK, stsBody),
			qwenGatewayTestResponse(http.StatusOK, ""),
		},
	}
	svc := &GatewayService{httpUpstream: upstream}
	attachment := qwenLocalAttachment{
		Filename:    "inline-image-1.jpg",
		ContentType: "image/jpeg",
		Data:        []byte("image-bytes"),
	}

	remote, err := svc.uploadQwenAttachment(context.Background(), nil, &Account{}, "https://chat.qwen.ai", "token", "", "", attachment)
	if err != nil {
		t.Fatalf("uploadQwenAttachment returned error: %v", err)
	}
	if len(upstream.requests) != 2 {
		t.Fatalf("expected getstsToken and OSS PUT only, got %d requests", len(upstream.requests))
	}
	if got := upstream.requests[1].Method; got != http.MethodPut {
		t.Fatalf("second request method = %s, want PUT", got)
	}
	file, ok := remote["file"].(map[string]any)
	if !ok {
		t.Fatalf("remote file missing: %+v", remote)
	}
	meta, ok := file["meta"].(map[string]any)
	if !ok {
		t.Fatalf("remote meta missing: %+v", file)
	}
	parseMeta, ok := meta["parse_meta"].(map[string]any)
	if !ok || parseMeta["parse_status"] != "success" {
		t.Fatalf("unexpected parse_meta: %+v", meta["parse_meta"])
	}
}

func TestQwenSSEPayloadToOpenAIChunksSkipsThinkingSummary(t *testing.T) {
	t.Parallel()

	payload := `{"choices":[{"delta":{"role":"assistant","content":"","phase":"thinking_summary","status":"typing"}}]}`
	if got := qwenSSEPayloadToOpenAIChunks(payload, "qwen3.6plus"); len(got) != 0 {
		t.Fatalf("expected no chunks for thinking summary, got %+v", got)
	}
}

func TestQwenSSEPayloadToOpenAIChunksAnswer(t *testing.T) {
	t.Parallel()

	payload := `{"choices":[{"delta":{"role":"assistant","content":"Hi there","phase":"answer","status":"typing"}}],"response_id":"resp-1","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3},"timestamp":123}`
	got := qwenSSEPayloadToOpenAIChunks(payload, "qwen3.6plus")
	if len(got) != 2 {
		t.Fatalf("expected answer chunk plus usage chunk, got %d", len(got))
	}
	if got[0].Choices[0].Delta.Content == nil || *got[0].Choices[0].Delta.Content != "Hi there" {
		t.Fatalf("unexpected answer chunk: %+v", got[0])
	}
	if got[1].Usage == nil || got[1].Usage.TotalTokens != 3 {
		t.Fatalf("unexpected usage chunk: %+v", got[1])
	}
}

func TestQwenSSEPayloadToOpenAIChunksExtraContent(t *testing.T) {
	t.Parallel()

	payload := `{"choices":[{"delta":{"role":"assistant","phase":"answer_finished","extra":{"content":"Hi from extra"}}}]}`
	got := qwenSSEPayloadToOpenAIChunks(payload, "qwen3.6-plus")
	if len(got) != 1 {
		t.Fatalf("expected content chunk, got %d", len(got))
	}
	if got[0].Choices[0].Delta.Content == nil || *got[0].Choices[0].Delta.Content != "Hi from extra" {
		t.Fatalf("unexpected content chunk: %+v", got[0])
	}
	if got[0].Choices[0].FinishReason == nil || *got[0].Choices[0].FinishReason != "stop" {
		t.Fatalf("expected stop finish reason, got %+v", got[0].Choices[0].FinishReason)
	}
}

func TestQwenSSEPayloadToOpenAIChunksDoesNotEmitPhaseAsContent(t *testing.T) {
	t.Parallel()

	payload := `{"choices":[{"delta":{"role":"assistant","phase":"answer","status":"typing"}}]}`
	if got := qwenSSEPayloadToOpenAIChunks(payload, "qwen3.6-plus"); len(got) != 0 {
		t.Fatalf("expected no chunk for phase-only payload, got %+v", got)
	}
}

func contentFromChatMessage(msg apicompat.ChatMessage, out *string) error {
	return json.Unmarshal(msg.Content, out)
}
