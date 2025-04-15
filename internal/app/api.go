package app

import "go.orx.me/apps/hyper-sync/internal/dao"

type ApiServer struct {
	dao *dao.MongoDAO
}

func NewApiServer(dao *dao.MongoDAO) (*ApiServer, error) {
	return &ApiServer{
		dao: dao,
	}, nil
}
