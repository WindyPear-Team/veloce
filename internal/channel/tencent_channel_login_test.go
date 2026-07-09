package channel

import "testing"

func TestParseTencentChannelCLIJSONWithPromptAndTrailingStderr(t *testing.T) {
	raw := "stdout:\n正在请求授权码...\n" +
		`{"data":{"expires_in_s":339,"message":"请扫描二维码或打开授权链接完成登录","qr_code":"iVBORw0KGgo=","status":"pending_authorization","verification_uri":"https://connect.qq.com/open-platform/device-bind?device_code=abc"},"success":true}` +
		"\n\nstderr:\nzerolog: could not write event"

	payload := parseTencentChannelCLIJSON(raw)
	response := tencentChannelLoginStartResponse(payload)
	if got := stringFromMap(response, "qr_data_url"); got != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("qr_data_url = %q", got)
	}
	if got := stringFromMap(response, "qrcode_url"); got == "" {
		t.Fatal("qrcode_url should be parsed")
	}
}

func TestTencentChannelGatewayPollPayloadForcesJSON(t *testing.T) {
	payload := tencentChannelGatewayPollPayload(map[string]interface{}{})
	if payload["json"] != true {
		t.Fatalf("json flag = %#v", payload["json"])
	}
	flags, ok := payload["cli_flags"].([]string)
	if !ok || len(flags) != 1 || flags[0] != "--json" {
		t.Fatalf("cli_flags = %#v", payload["cli_flags"])
	}
}

func TestParseTencentChannelGatewayPollResultWrappedStdout(t *testing.T) {
	raw := "stdout:\n" +
		`{"success":true,"data":{"events":[{"content":"hello","guild_id":"g1"}],"cursor":"c1","watermark":"w1"}}` +
		"\n\nstderr:\nnotice"

	result := parseTencentChannelGatewayPollResult(raw)
	if len(result.Events) != 1 {
		t.Fatalf("events length = %d", len(result.Events))
	}
	if result.Cursor != "c1" || result.Watermark != "w1" {
		t.Fatalf("cursor/watermark = %q/%q", result.Cursor, result.Watermark)
	}
}
