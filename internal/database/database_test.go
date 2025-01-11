package database

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
)

func TestSetDocument(t *testing.T) {
	project = "speedy-victory-336109"
	client := New()
	data := map[string]interface{}{
		"name": "John Doe",
		"age":  31,
	}
	resp, err := client.SetDocument("test", "aaa", data)
	if err != nil {
		log.Fatalf("failed set doc: %v", err)
	}
	log.Println(resp)

}

func TestListDocuments(t *testing.T) {
	project = "speedy-victory-336109"
	client := New()
	resp, err := client.ListDocuments("rok_match")
	if err != nil {
		log.Fatalf("failed set doc: %v", err)
	}
	log.Println(resp)
}

func TestLoaddata(t *testing.T) {
	game := "afk"
	file_path := fmt.Sprintf("../../testdata/%s.match", game)
	project = "speedy-victory-336109"
	client := New()

	file, err := os.Open(file_path) // 替换为你的文件路径
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var data = make(map[string]interface{})
		line := scanner.Text() // 获取当前行的文本
		result := strings.Split(line, "|")
		array := strings.Split(result[1], ",")
		data["label"] = result[0]
		data["match_rules"] = array
		resp, err := client.AddData(fmt.Sprintf("%s_match", game), data)
		if err != nil {
			log.Fatalf("failed set doc: %v", err)
		}
		log.Println(resp)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

}

func TestAddData(t *testing.T) {
	project = "speedy-victory-336109"
	client := New()
	data := map[string]interface{}{
		"name": "John Doe",
		"age":  31,
	}
	resp, err := client.AddData("test", data)
	if err != nil {
		log.Fatalf("failed add data to Firestore: %v", err)
	}
	log.Println(resp)
}
