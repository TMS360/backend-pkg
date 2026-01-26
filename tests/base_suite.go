package tests

import (
	"context"
	"log/slog"
	"os"

	"github.com/TMS360/backend-pkg/client/postgresql"
	"github.com/TMS360/backend-pkg/config"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/TMS360/backend-pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type BaseSuite struct {
	suite.Suite
	Log       *slog.Logger
	DB        *gorm.DB
	Tx        *gorm.DB
	Tm        tmsdb.TransactionManager
	Ctx       context.Context
	UserID    uuid.UUID
	CompanyID uuid.UUID
}

func (s *BaseSuite) SetupSuite(dbCfg config.PostgresSQLConfig) {
	s.Log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	db, err := postgresql.NewClient(dbCfg)
	s.Require().NoError(err)

	db.Use(&tmsdb.TenantScopePlugin{})

	s.UserID = uuid.New()
	s.CompanyID = uuid.New()
	s.Ctx = middleware.WithActor(context.Background(), s.UserID, &consts.UserClaims{
		UserID:    s.UserID,
		CompanyID: utils.Pointer(s.CompanyID),
	})

	s.DB = db.WithContext(s.Ctx)
}

func (s *BaseSuite) SetupTest() {
	s.Tx = s.DB.Begin()
	s.Tm = tmsdb.NewGormTransactionManager(s.Tx, "test")
}

func (s *BaseSuite) TearDownTest() {
	if s.Tx != nil {
		s.Tx.Rollback()
	}
}
