package dao

import "go.mongodb.org/mongo-driver/v2/mongo"

type MongoDAO struct {
	Client   *mongo.Client
	Database string
}

func NewMongoDAO(client *mongo.Client, database string) *MongoDAO {
	return &MongoDAO{
		Client:   client,
		Database: database,
	}
}
