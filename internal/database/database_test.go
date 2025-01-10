package database

import (
	"log"
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
