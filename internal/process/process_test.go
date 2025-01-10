package process

import (
	"testing"
)

func TestProcess(t *testing.T) {
	project = "speedy-victory-336109"
	location = "us-central1"
	model = "gemini-1.5-pro-002"
	temperatureStr = "1.0"
	prompt := "描述视频"
	url := "https://storage.googleapis.com/pwm-lilith/dap-videos/5e123b68-714e-11ee-9173-86cdd1be383b.mp4"
	resp, err := New().Process(prompt, url)
	if err != nil {
		t.Fatalf("failed process: %v", err)
	}
	t.Log(resp)
}
