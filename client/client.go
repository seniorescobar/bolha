package client

import (
	"io"
	"net/http"
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
	Price       int
	CategoryId  int
	Images      []io.Reader
}

// ActiveAd represents an active (uploaded) ad from bolha.com
type ActiveAd struct {
	Id    int64
	Order int
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

	if err := client.login(u); err != nil {
		return nil, err
	}

	return client, nil
}

// GET
// GetActiveAds returns active ads
func (c *Client) GetActiveAds() ([]*ActiveAd, error) {
	return c.getActiveAds()
}

// UPLOAD
// UploadAd uploads a single ad
func (c *Client) UploadAd(ad *Ad) (int64, error) {
	return c.uploadAd(ad)
}

// REMOVE
// RemoveAd removes a single ad provided by an id
func (c *Client) RemoveAd(id int64) error {
	return c.removeAds([]int64{id})
}

// RemoveAds removes multiple ads provided by ids
func (c *Client) RemoveAds(ids []int64) error {
	return c.removeAds(ids)
}

// RemoveAllAds removes all ads found on a user's account
func (c *Client) RemoveAllAds() error {
	activeAds, err := c.getActiveAds()
	if err != nil {
		return err
	}

	ids := make([]int64, len(activeAds))
	for i := range activeAds {
		ids[i] = activeAds[i].Id
	}

	return c.removeAds(ids)
}
