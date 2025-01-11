package process

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"cloud.google.com/go/vertexai/genai"
	mygenai "github.com/hellof20/pwm-gcp-genai"
)

type Service interface {
	ProcessURL(prompt, url string) (string, error)
	ProcessCaption(prompt, caption string) (string, error)
}

type service struct {
	client *mygenai.GeminiAPI
}

var (
	project        = os.Getenv("PROJECT_ID")
	location       = os.Getenv("LOCATION")
	model          = os.Getenv("MODEL")
	temperatureStr = os.Getenv("TEMPERATURE")
)

func New() Service {
	if project == "" || location == "" || model == "" || temperatureStr == "" {
		log.Fatal("Missing required environment variables")
	}

	temperature, err := strconv.ParseFloat(temperatureStr, 64)
	if err != nil {
		log.Fatalf("Invalid TEMPERATURE value: %v", err)
	}

	client := mygenai.NewGeminiAPI(location, project, model, float32(temperature), 3, 1)
	return &service{client: client}
}

func (s *service) ProcessURL(prompt, url string) (string, error) {
	s.client.ResponseMIMEType = "application/json"
	s.client.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject, Properties: map[string]*genai.Schema{
			"label": {Type: genai.TypeString},
		},
	}
	resp, err := s.client.Invoke(
		&mygenai.TextInput{Text: prompt},
		&mygenai.BlobInput{Path: url},
	)
	if err != nil {
		log.Printf("failed invoke: %v", err)
		return "", fmt.Errorf("failed to invoke Gemini API: %w", err)
	}
	return resp, nil
}

func (s *service) ProcessCaption(prompt, caption string) (string, error) {
	s.client.ResponseMIMEType = "application/json"
	s.client.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject, Properties: map[string]*genai.Schema{
			"label": {Type: genai.TypeString},
		},
	}
	resp, err := s.client.Invoke(
		&mygenai.TextInput{Text: prompt},
		&mygenai.TextInput{Text: caption},
	)
	if err != nil {
		log.Printf("failed invoke: %v", err)
		return "", fmt.Errorf("failed to invoke Gemini API: %w", err)
	}
	return resp, nil
}
