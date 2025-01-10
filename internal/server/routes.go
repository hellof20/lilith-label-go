package server

import (
	"encoding/json"
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

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/", s.GetMsgHandler)

	mux.HandleFunc("/labels", s.AddDataHandler)

	// Wrap the mux with CORS middleware
	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Replace "*" with specific origins if needed
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "false") // Set to "true" if credentials are required

		// Handle preflight OPTIONS requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Proceed with the next handler
		next.ServeHTTP(w, r)
	})
}

func (s *Server) GetMsgHandler(w http.ResponseWriter, r *http.Request) {
	msgID := r.URL.Query().Get("msg_id")
	if msgID == "" {
		http.Error(w, "Missing msg_id parameter", http.StatusBadRequest)
		return
	}

	resp, _ := s.db.GetDocument("videolabel", msgID)

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(jsonResp); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func (s *Server) AddDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 解析 JSON 数据
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
		return
	}
	data["status"] = "queued"

	// 使用从请求中获取的数据设置文档
	resp, err := s.db.AddData("videolabel", data)
	if err != nil {
		http.Error(w, "Failed to add data", http.StatusInternalServerError)
		return
	}
	doc_id := resp["msg_id"]

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Failed to marshal set document response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(jsonResp); err != nil {
		log.Printf("Failed to write response: %v", err)
	}

	// 异步处理请求
	go func() {
		start := time.Now() //开始时间
		var totalLabels []string
		log.Printf("异步处理: %s", doc_id)

		game := data["game"].(string)
		url := data["url"].(string)
		log.Println(game, url)

		//获取视频字幕
		resp, err := s.api.Process("提取视频字幕，在一行内输出，原始语言保持不变", url)
		if err != nil {
			log.Printf("Failed to process: %v", err)
			s.setFirestoreDocFailed(doc_id, data, err)
			return
		}
		var caption Response
		json.Unmarshal([]byte(resp), &caption)
		log.Println(caption.Label)

		// 从firestore获取游戏match规则
		match_rules, err := s.db.GetDocument(game, "match")
		if err != nil {
			log.Printf("Failed to get match rules: %v", err)
			s.setFirestoreDocFailed(doc_id, data, err)
			return
		}
		// 执行匹配逻辑
		match_results := processCaption(caption.Label, match_rules)
		totalLabels = append(totalLabels, match_results...)

		// 从firestore获取游戏对应prompt
		prompts, err := s.db.GetDocument(game, "prompt")
		if err != nil {
			log.Printf("Failed to get prompt: %v", err)
		}

		// 调用Gemini接口
		var wg sync.WaitGroup
		wg.Add(len(prompts))
		for _, v := range prompts {
			prompt := v.(string)
			go func(p string) {
				defer wg.Done()
				resp, err := s.api.Process(p, url)
				if err != nil {
					log.Printf("Failed to process: %v", err)
					s.setFirestoreDocFailed(doc_id, data, err)
					return
				}
				var resp_1 Response
				json.Unmarshal([]byte(resp), &resp_1)
				for _, v := range splitString(resp_1.Label) {
					if v != "other" {
						totalLabels = append(totalLabels, v)
					}
				}
				log.Printf("Prompt: %s, Label: %s", p, resp_1.Label)
			}(prompt)
		}
		wg.Wait()

		//处理完写入数据到Firestore
		data["status"] = "done"
		data["labels"] = totalLabels
		//当前时间，格式为2025-01-10 16:27:00
		currentTime := time.Now().Format("2006-01-02 15:04:05")
		data["date"] = currentTime
		end := time.Now() //结束时间
		spend_time := end.Sub(start)
		data["spend_time"] = spend_time.String()
		log.Printf("处理耗时: %v", spend_time)
		s.db.SetDocument("videolabel", doc_id, data)
		log.Println(data)
		log.Println("处理完成")
	}()
}

// create a func , split string by comma ,return []string
func splitString(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// create a func, set firestore doc , status failed
func (s *Server) setFirestoreDocFailed(docID string, data map[string]interface{}, err error) {
	data["status"] = "failed"
	data["error"] = err.Error()
	s.db.SetDocument("videolabel", docID, data)
}

func processCaption(caption string, match_rules map[string]interface{}) []string {
	totalLabels := []string{}

	for k, v := range match_rules {
		if vs, ok := v.(string); ok {
			// 使用逗号分割字符串
			values := strings.Split(vs, ",")
			for _, singleValue := range values {
				// 去除首尾空格
				trimmedValue := strings.TrimSpace(singleValue)
				if trimmedValue != "" && strings.Contains(caption, trimmedValue) {
					totalLabels = append(totalLabels, k)
					goto NextRule
				}
			}
		}
	NextRule:
	}
	return totalLabels
}
