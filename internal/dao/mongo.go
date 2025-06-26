package dao

import (
	bmongo "butterfly.orx.me/core/store/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoDAO struct {
	Client   *mongo.Client
	Database string
}

func NewMongoClient() *mongo.Client {
	return bmongo.GetClient("main")
}

func NewMongoDAO(client *mongo.Client) *MongoDAO {
	return &MongoDAO{
		Client:   client,
		Database: "hypersync",
	}
}

func NewPostDao(client *mongo.Client) PostDao {
	return NewMongoDAO(client)
}

func NewSocialConfigDao(client *mongo.Client) SocialConfigDao {
	return NewMongoDAO(client)
}
