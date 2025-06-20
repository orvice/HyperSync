package dao

import (
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoDAO struct {
	Client   *mongo.Client
	Database string
}

func NewMongoClient() (*mongo.Client, error) {
	return nil, nil
}

func NewMongoDAO(client *mongo.Client, database string) *MongoDAO {
	return &MongoDAO{
		Client:   client,
		Database: database,
	}
}
