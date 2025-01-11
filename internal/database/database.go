package database

import (
	"context"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	_ "github.com/joho/godotenv/autoload"
	"google.golang.org/api/iterator"
)

// Service represents a service that interacts with a database.
type Service interface {
	SetDocument(col, doc string, data interface{}) (map[string]string, error)
	GetDocument(col, doc string) (map[string]interface{}, error)
	AddData(col string, data interface{}) (map[string]string, error)
	ListDocuments(col string) ([]map[string]interface{}, error)
}

type service struct {
	db *firestore.Client
}

var (
	project = os.Getenv("PROJECT_ID")
)

func New() Service {
	client, err := firestore.NewClient(context.Background(), project)
	if err != nil {
		log.Fatal(err)
	}
	return &service{
		db: client,
	}
}

func (s *service) SetDocument(col, doc string, data interface{}) (map[string]string, error) {
	_, err := s.db.Collection(col).Doc(doc).Set(context.Background(), data)
	if err != nil {
		return nil, err
	}
	return map[string]string{"status": "success"}, nil
}

func (s *service) GetDocument(col, doc string) (map[string]interface{}, error) {
	dsnap, err := s.db.Collection(col).Doc(doc).Get(context.Background())
	if err != nil {
		return nil, err
	}
	data := dsnap.Data()
	return data, nil
}

func (s *service) ListDocuments(col string) ([]map[string]interface{}, error) {
	ctx := context.Background()
	iter := s.db.Collection(col).Documents(ctx)
	defer iter.Stop()
	var alldata []map[string]interface{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		data := doc.Data()
		alldata = append(alldata, data)
	}
	return alldata, nil
}

func (s *service) AddData(col string, data interface{}) (map[string]string, error) {
	doc_id, _, err := s.db.Collection(col).Add(context.Background(), data)
	if err != nil {
		return nil, err
	}
	return map[string]string{"msg_id": doc_id.ID}, nil
}
