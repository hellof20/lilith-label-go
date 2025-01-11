package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Response struct {
	Label string `json:"label"`
}

type DocData struct {
	Status    string                 `json:"status"`
	Labels    []string               `json:"labels"`
	Date      string                 `json:"date"`
	SpendTime string                 `json:"spend_time"`
	Error     string                 `json:"error,omitempty"`
	Game      string                 `json:"game"`
	URL       string                 `json:"url"`
	Lang      string                 `json:"lang"`
	MsgID     string                 `json:"msg_id,omitempty"`
	Other     map[string]interface{} `json:"-"` // 忽略其他字段
}

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.GetMsgHandler)
	mux.HandleFunc("/labels", s.AddDataHandler)
	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "false")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) GetMsgHandler(w http.ResponseWriter, r *http.Request) {
	msgID := r.URL.Query().Get("msg_id")
	if msgID == "" {
		s.respondWithError(w, http.StatusBadRequest, "Missing msg_id parameter")
		return
	}
	resp, err := s.db.GetDocument("videolabel", msgID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get document: %v", err))
		return
	}

	s.respondWithJSON(w, http.StatusOK, resp)
}

func (s *Server) AddDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 读取并解析请求体
	docData, err := s.parseRequestData(r)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 添加数据到数据库并获取文档 ID
	docData.Status = "queued"
	resp, err := s.db.AddData("videolabel", docData)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add data: %v", err))
		return
	}
	docData.MsgID = resp["msg_id"]

	s.respondWithJSON(w, http.StatusCreated, resp)

	// 启动异步处理
	go s.processDataAsync(docData)
}

// 解析请求体并返回 DocData
func (s *Server) parseRequestData(r *http.Request) (*DocData, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("failed to read request body")
	}
	defer r.Body.Close()

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, errors.New("failed to unmarshal request body")
	}

	docData := &DocData{
		Other: make(map[string]interface{}),
	}

	if game, ok := data["game"].(string); ok {
		docData.Game = game
	} else {
		return nil, errors.New("invalid 'game' field")
	}
	if url, ok := data["url"].(string); ok {
		docData.URL = url
	} else {
		return nil, errors.New("invalid 'url' field")
	}
	if lang, ok := data["lang"].(string); ok {
		docData.Lang = lang
	} else {
		return nil, errors.New("invalid 'lang' field")
	}

	for k, v := range data {
		if k != "game" && k != "url" && k != "lang" {
			docData.Other[k] = v
		}
	}

	return docData, nil
}

// 异步处理数据
func (s *Server) processDataAsync(docData *DocData) {
	start := time.Now()
	log.Printf("异步处理开始: %s", docData.MsgID)

	// 1. 获取视频字幕
	log.Println("1. 获取视频字幕")
	caption, err := s.fetchCaption(docData.URL)
	if err != nil {
		s.setFirestoreDocFailed(docData, err)
		return
	}
	log.Println(caption)

	// 2. 执行匹配逻辑
	log.Println("2. 执行匹配逻辑")
	totalLabels, err := s.executeMatchingLogic(docData.Game, caption)
	if err != nil {
		s.setFirestoreDocFailed(docData, err)
		return
	}
	log.Println(totalLabels)

	// 3. 获取和处理 Text prompts
	log.Println("3. 获取和处理 Text prompts")
	textLabels, err := s.processTextPrompts(docData.Game, docData.Lang, caption)
	if err != nil {
		s.setFirestoreDocFailed(docData, err)
		return
	}
	totalLabels = append(totalLabels, textLabels...)
	log.Println(totalLabels)

	// 4. 获取和处理 Video Prompts
	log.Println("4. 获取和处理 Video Prompts")
	videoLabels, err := s.processVideoPrompts(docData.Game, docData.Lang, docData.URL)
	if err != nil {
		s.setFirestoreDocFailed(docData, err)
		return
	}
	totalLabels = append(totalLabels, videoLabels...)
	log.Println(totalLabels)

	// 5. 更新 Firestore 文档
	log.Println("5. 更新 Firestore 文档")
	docData.Status = "done"
	docData.Labels = totalLabels
	docData.Date = time.Now().Format("2006-01-02 15:04:05")
	docData.SpendTime = time.Since(start).String()

	if err := s.updateFirestoreDocument(docData); err != nil {
		log.Printf("Failed to update Firestore document: %v", err)
	}

	log.Printf("异步处理完成: %s, 耗时: %s", docData.MsgID, docData.SpendTime)

}

// 获取视频字幕
func (s *Server) fetchCaption(url string) (string, error) {
	resp, err := s.api.ProcessURL("获取视频脚本，加上标点符号后在一行内输出，原始语言保持不变", url)
	if err != nil {
		return "", fmt.Errorf("failed to process URL: %w", err)
	}
	var caption Response
	if err := json.Unmarshal([]byte(resp), &caption); err != nil {
		return "", fmt.Errorf("failed to unmarshal caption: %w", err)
	}
	return strings.ToLower(caption.Label), nil
}

// 执行匹配逻辑
func (s *Server) executeMatchingLogic(game string, caption string) ([]string, error) {
	matchRules, err := s.db.ListDocuments(fmt.Sprintf("%s_match", game))
	if err != nil {
		return nil, fmt.Errorf("failed to get match rules: %w", err)
	}

	var totalLabels []string
	for _, v := range matchRules {
		label, ok := v["label"].(string)
		if !ok {
			log.Printf("label is not a string: %v", v["label"])
			continue
		}
		rules, ok := v["match_rules"].([]interface{})
		if !ok {
			log.Printf("match_rules is not an array: %v", v["match_rules"])
			continue
		}
		for _, r := range rules {
			if strings.Contains(caption, r.(string)) {
				totalLabels = append(totalLabels, label)
			}
		}
	}
	return totalLabels, nil
}

// 处理 Text prompts
func (s *Server) processTextPrompts(game, lang, caption string) ([]string, error) {
	textPrompts, err := s.db.ListDocuments(fmt.Sprintf("%s_%s_text_prompts", game, lang))
	if err != nil {
		return nil, fmt.Errorf("failed to get text prompts: %w", err)
	}

	var totalLabels []string
	var wg sync.WaitGroup
	wg.Add(len(textPrompts))

	labelChan := make(chan string, len(textPrompts))

	for _, v := range textPrompts {
		prompt, ok := v["content"].(string)
		if !ok {
			log.Printf("prompt is not a string: %v", v["content"])
			continue
		}
		go func(p string) {
			defer wg.Done()
			resp, err := s.api.ProcessCaption(p, caption)
			if err != nil {
				log.Printf("Failed to process caption: %v", err)
				return
			}
			var resp_1 Response
			if err := json.Unmarshal([]byte(resp), &resp_1); err != nil {
				log.Printf("Failed to unmarshal response: %v", err)
				return
			}
			if resp_1.Label != "other" {
				labelChan <- resp_1.Label
			}
			log.Printf("Text Prompt: %s, Label: %s", p, resp_1.Label)
		}(prompt)
	}
	wg.Wait()
	close(labelChan)
	for label := range labelChan {
		totalLabels = append(totalLabels, label)
	}
	return totalLabels, nil
}

// 处理 Video Prompts
func (s *Server) processVideoPrompts(game, lang, url string) ([]string, error) {
	videoPrompts, err := s.db.ListDocuments(fmt.Sprintf("%s_%s_video_prompts", game, lang))
	if err != nil {
		return nil, fmt.Errorf("failed to get video prompts: %w", err)
	}

	var totalLabels []string
	var wg sync.WaitGroup
	wg.Add(len(videoPrompts))

	labelChan := make(chan []string, len(videoPrompts))

	for _, v := range videoPrompts {
		videoPrompt, ok := v["content"].(string)
		if !ok {
			log.Printf("prompt is not a string: %v", v["content"])
			continue
		}
		go func(p string) {
			defer wg.Done()
			resp, err := s.api.ProcessURL(p, url)
			if err != nil {
				log.Printf("Failed to process URL: %v", err)
				return
			}
			var resp_1 Response
			if err := json.Unmarshal([]byte(resp), &resp_1); err != nil {
				log.Printf("Failed to unmarshal response: %v", err)
				return
			}

			labels := splitString(resp_1.Label)
			var filteredLabels []string
			for _, v := range labels {
				if v != "other" {
					filteredLabels = append(filteredLabels, v)
				}
			}
			labelChan <- filteredLabels
			log.Printf("Video Prompt: %s, Label: %s", p, resp_1.Label)

		}(videoPrompt)
	}

	wg.Wait()
	close(labelChan)
	for labels := range labelChan {
		totalLabels = append(totalLabels, labels...)
	}

	return totalLabels, nil
}

// 更新 Firestore 文档
func (s *Server) updateFirestoreDocument(docData *DocData) error {
	updatedData := map[string]interface{}{
		"status":     docData.Status,
		"labels":     docData.Labels,
		"date":       docData.Date,
		"spend_time": docData.SpendTime,
		"game":       docData.Game,
		"url":        docData.URL,
		"lang":       docData.Lang,
	}
	if docData.Error != "" {
		updatedData["error"] = docData.Error
	}
	for k, v := range docData.Other {
		updatedData[k] = v
	}

	_, err := s.db.SetDocument("videolabel", docData.MsgID, updatedData)
	if err != nil {
		return fmt.Errorf("failed to set document: %w", err)
	}
	return nil
}

// 设置 Firestore 文档状态为失败
func (s *Server) setFirestoreDocFailed(docData *DocData, err error) {
	docData.Status = "failed"
	docData.Error = err.Error()
	if updateErr := s.updateFirestoreDocument(docData); updateErr != nil {
		log.Printf("Failed to update Firestore document with error: %v, original error: %v", updateErr, err)
	}
}

// 响应 JSON 数据
func (s *Server) respondWithJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// 响应错误信息
func (s *Server) respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := map[string]string{"error": message}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}

func splitString(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}
