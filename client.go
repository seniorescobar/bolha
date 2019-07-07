package client

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

// Record represents a record consisting of user and ads
type Record struct {
	User *User
	Ads  []*Ad
}

// User represents a user of bolha.com
type User struct {
	Username string
	Password string
}

// Ad represents an ad from bolha.com
type Ad struct {
	Title       string
	Description string
	Price       string
	CategoryId  string
	Images      []string
}

// CLIENT
// Client represents a bolha client
type Client struct {
	httpClient *http.Client
}

// New creates a new bolha client
func New(u *User) (*Client, error) {
	httpClient, err := getHttpClient()
	if err != nil {
		return nil, err
	}

	client := &Client{
		httpClient: httpClient,
	}

	client.allowRedirects(false)

	if err := client.logIn(u); err != nil {
		return nil, err
	}

	return client, nil
}

// UPLOAD
// UploadAd uploads a single ad
func (c *Client) UploadAd(ad *Ad) {
	c.UploadAds([]*Ad{ad})
}

// UploadAds uploads multiple ads
func (c *Client) UploadAds(ads []*Ad) {
	for _, ad := range ads {
		if err := c.uploadAd(ad); err != nil {
			log.WithFields(log.Fields{
				"err": err,
			}).Error("error upload ad")
		}
	}
}

// REMOVE
// RemoveAd removes a single ad provided by an id
func (c *Client) RemoveAd(id string) error {
	return c.removeAds([]string{id})
}

// RemoveAds removes multiple ads provided by ids
func (c *Client) RemoveAds(ids []string) error {
	return c.removeAds(ids)
}

// RemoveAllAds removes all ads found on a user's account
func (c *Client) RemoveAllAds() error {
	ids, err := c.getAdIds()
	if err != nil {
		return err
	}

	return c.removeAds(ids)
}
