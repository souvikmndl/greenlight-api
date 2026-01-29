package main

import (
	"context"
	"database/sql"
	"flag"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/souvikmndl/greenlight-api/internal/data"
	"github.com/souvikmndl/greenlight-api/internal/mailer"
)

const version = "1.0.0"

type (
	config struct {
		port int
		env  string
		db   struct {
			dsn          string
			maxOpenConns int
			maxIdleConns int
			maxIdleTime  time.Duration
		}
		limiter struct {
			rps     float64
			burst   int
			enabled bool
		}
		smtp struct {
			host     string
			port     int
			username string
			password string
			sender   string
		}
		cors struct {
			trustedOrigins []string
		}
	}

	application struct {
		config config
		logger *slog.Logger
		models data.Models
		mailer *mailer.Mailer
		wg     sync.WaitGroup
	}
)

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 4000, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")

	// default maxOpenConns for PSQL is 100, and ideally maxIdleConns == maxOpenConns
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("GREENLIGHT_DB_DSN"), "PostgreSQL DSN")
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-cons", 25, "PostgreSQL max idle connections")
	flag.DurationVar(&cfg.db.maxIdleTime, "db-max-idle-time", 15*time.Minute, "PostgreSQL max connection idle time")

	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

	flag.StringVar(&cfg.smtp.host, "smtp-host", "sandbox.smtp.mailtrap.io", "SMTP host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", 25, "SMTP port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", "1142b361cbb2c4", "SMTP username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", "c910bb46b0730d", "SMTP password")
	flag.StringVar(&cfg.smtp.sender, "smtp-sender", "Greenlight <souvik@example.com>", "SMTP sender")

	flag.Func("cors-trusted-origins", "trusted CORS origins (space seperated)", func(val string) error {
		// Fields(s) splits the string s on spaces and returns a list/slice
		cfg.cors.trustedOrigins = strings.Fields(val)
		return nil
	})

	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := openDB(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("db connection established")

	mailer, err := mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
		mailer: mailer,
	}

	// mux := http.NewServeMux()
	// mux.HandleFunc("/v1/healthcheck", app.healthCheckHandler)
	err = app.serve()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)
	db.SetConnMaxIdleTime(cfg.db.maxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// ctx has timeout of 5s, PingContext will try to establish a connection with a timeout of 5s
	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
