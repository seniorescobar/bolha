package postgres

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db *sql.DB
}

type Conf struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

func New(conf *Conf) (*PostgresDB, error) {
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		conf.Host, conf.Port, conf.User, conf.Password, conf.DBName))
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return &PostgresDB{db}, nil
}

type Record struct {
	User
	Ad
}

type User struct {
	Username string
	Password string
}

type Ad struct {
	Title       string
	Description string
	Price       int
	CategoryId  int
	Images      []string
}

// CreateUser inserts new user record into "user" table
func (pdb *PostgresDB) CreateUser(ctx context.Context, u *User) error {
	if _, err := pdb.db.ExecContext(ctx, `INSERT INTO "user"("username", "password") VALUES($1, $2)`, u.Username, u.Password); err != nil {
		return err
	}

	return nil
}

// ListUsers returns records from "user" table
func (pdb *PostgresDB) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := pdb.db.QueryContext(ctx, `SELECT "username", "password" FROM "user"`)
	if err != nil {
		return nil, err
	}

	users := make([]*User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.Username, &user.Password); err != nil {
			return nil, err
		}

		users = append(users, &user)
	}

	if err := rows.Close(); err != nil {
		return nil, err
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// ListActiveUsers returns records from "user" table if also found in "ad" table
func (pdb *PostgresDB) ListActiveUsers(ctx context.Context) ([]*User, error) {
	rows, err := pdb.db.QueryContext(ctx, `
		SELECT
			"username",
			"password"
		FROM "user"
		WHERE
			"username" IN (
				SELECT
					"a"."user_username"
				FROM "uploaded_ad" "ua"
				LEFT JOIN "ad" "a" ON "a"."id" = "ua"."ad_id"
			)
	`)
	if err != nil {
		return nil, err
	}

	users := make([]*User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.Username, &user.Password); err != nil {
			return nil, err
		}

		users = append(users, &user)
	}

	if err := rows.Close(); err != nil {
		return nil, err
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// GetRecord returns a record (user + ad) for a given uploaded_ad_id
func (pdb *PostgresDB) GetRecord(ctx context.Context, uploadedAdId int64) (*Record, error) {
	var record Record
	if err := pdb.db.QueryRowContext(ctx, `
		SELECT
			"u"."username",
			"u"."password",
			"a"."title",
			"a"."description",
			"a"."price",
			"a"."category_id"
		FROM "uploaded_ad" "ua"
		LEFT JOIN "ad" "a" ON "a"."id" = "ua"."ad_id"
		LEFT JOIN "user" "u" ON "u"."username" = "a"."user_username"
		WHERE
			"ua"."uploaded_ad_id" = $1
	`, uploadedAdId).Scan(&record.User.Username, &record.User.Password, &record.Ad.Title, &record.Ad.Description, &record.Ad.Price, &record.Ad.CategoryId); err != nil {
		return nil, err
	}

	return &record, nil
}

// AddAd inserts new ad record into "ad"
func (pdb *PostgresDB) AddAd(ctx context.Context, username string, ad *Ad) error {
	tx, err := pdb.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// add ad
	var adId int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO "ad"("user_username", "title", "description", "price", "category_id") VALUES($1, $2, $3, $4, $5) RETURNING id`, username, ad.Title, ad.Description, ad.Price, ad.CategoryId).Scan(&adId); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return rerr
		}
		return err
	}

	// add images
	for _, img := range ad.Images {
		if err := pdb.addImage(ctx, tx, adId, img); err != nil {
			if rerr := tx.Rollback(); rerr != nil {
				return rerr
			}
			return err
		}
	}

	return tx.Commit()
}

func (pdb *PostgresDB) addImage(ctx context.Context, tx *sql.Tx, adId int64, location string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO "image"("ad_id", "location") VALUES($1, $2)`, adId, location); err != nil {
		return err
	}

	return nil
}

func (pdb *PostgresDB) Close() {
	pdb.db.Close()
}
