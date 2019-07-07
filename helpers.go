package client

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	regexImageId = `"id":"([a-z0-9\-]*)"`
)

func (c *Client) uploadAd(ad *Ad) error {
	log.WithFields(log.Fields{
		"ad": ad,
	}).Info("uploading ad...")

	metaInfo, err := c.getAdMetaInfo(ad)
	if err != nil {
		return err
	}

	return c.publishAd(ad, metaInfo)
}

func (c *Client) removeAds(ids []string) error {
	log.WithFields(log.Fields{
		"ids": ids,
	}).Info("removing ads...")

	values := url.Values{
		"IDS": {
			strings.Join(ids, ","),
		},
	}

	req, err := http.NewRequest(http.MethodPost, "https://moja.bolha.com/adManager/ajaxRemoveActiveBulk", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}

	overwrite := map[string]string{
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "moja.bolha.com",
		"Origin":                    "https://moja.bolha.com",
		"Referer":                   "https://moja.bolha.com/oglasi",
		"Upgrade-Insecure-Requests": "1",
		"X-Requested-With":          "XMLHttpRequest",
	}
	headers := getDefaultHeaders(overwrite)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.New("ads not removed")
	}

	return nil
}

func getHttpClient() (*http.Client, error) {
	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout: time.Duration(3) * time.Minute,
		Jar:     cookieJar,
	}, nil
}

func (c *Client) allowRedirects(allow bool) {
	if allow {
		c.httpClient.CheckRedirect = nil
	} else {
		c.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

func (c *Client) logIn(u *User) error {
	values := url.Values{
		"username": {
			u.Username,
		},
		"password": {
			u.Password,
		},
		"rememberMe": {
			"true",
		},
	}

	req, err := http.NewRequest(http.MethodPost, "https://login.bolha.com/auth.php", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}

	overwrite := map[string]string{
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "login.bolha.com",
		"Origin":                    "http://www.bolha.com",
		"Referer":                   "http://www.bolha.com/",
		"Upgrade-Insecure-Requests": "1",
		"X-Requested-With":          "XMLHttpRequest",
		"X-Site":                    "http://www.bolha.com/",
	}
	headers := getDefaultHeaders(overwrite)

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("login failed for '%s' ('%s')", u.Username, u.Password)
	}

	return nil
}

func (c *Client) getAdIds() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://moja.bolha.com/oglasi", nil)
	if err != nil {
		return nil, err
	}

	overwrite := map[string]string{
		"Host":                      "moja.bolha.com",
		"Upgrade-Insecure-Requests": "1",
	}
	headers := getDefaultHeaders(overwrite)

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(`id="ad_(\d+)`)
	matches := r.FindAllStringSubmatch(string(body), -1)
	if matches == nil {
		return nil, errors.New("no ads found")
	}

	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m[1]
	}

	return ids, nil
}

func (c *Client) getAdMetaInfo(ad *Ad) (map[string]string, error) {
	values := url.Values{
		"categoryId": {
			ad.CategoryId,
		},
	}

	req, err := http.NewRequest(http.MethodPost, "http://objava-oglasa.bolha.com/izbor_paketa.php", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}

	overwrite := map[string]string{
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "objava-oglasa.bolha.com",
		"Origin":                    "http://objava-oglasa.bolha.com",
		"Referer":                   "http://objava-oglasa.bolha.com/",
		"Upgrade-Insecure-Requests": "1",
	}
	headers := getDefaultHeaders(overwrite)

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	c.allowRedirects(true)
	defer func() {
		c.allowRedirects(false)
	}()

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	regex := map[string]*regexp.Regexp{
		"submitTakoj":         regexp.MustCompile(`<input type="hidden" name="submitTakoj" id="submitTakoj" value="(.*?)" />`),
		"listItemId":          regexp.MustCompile(`<input type="hidden" name="listItemId" id="listItemId" value="(.*?)" />`),
		"lPreverjeni":         regexp.MustCompile(`<input type="hidden" name="lPreverjeni" id="lPreverjeni" value="(.*?)" />`),
		"lShop":               regexp.MustCompile(`<input type="hidden" name="lShop" id="lShop" value="(.*?)">`),
		"uploader_id":         regexp.MustCompile(`<input type="hidden" name="uploader_id" id="uploader_id" value="(.*?)" />`),
		"novo":                regexp.MustCompile(`<input type="hidden" name="novo" value="(.*?)" />`),
		"adPlacementPrice":    regexp.MustCompile(`<input type="hidden" name="adPlacementPrice" id="adPlacementPrice" value="(.*?)" />`),
		"adPlacementDiscount": regexp.MustCompile(`<input type="hidden" name="adPlacementDiscount" id="adPlacementDiscount" value="(.*?)" />`),
		"nDays":               regexp.MustCompile(`<input type="hidden" name="nDays" value="(.*?)" />`),
		"spremeni":            regexp.MustCompile(`<input type="hidden" name="spremeni" value="(.*?)" />`),
		"new":                 regexp.MustCompile(`<input type="hidden" name="new" value="(.*?)" />`),
		"nKatID":              regexp.MustCompile(`<input name="nKatID" id="nKatID" type="hidden" size="5" value="(.*?)" />`),
		"nNadKatID":           regexp.MustCompile(`<input name="nNadKatID" id="nNadKatID" type="hidden" size="5" value="(.*?)" />`),
		"nMainKatID":          regexp.MustCompile(`<input name="nMainKatID" id="nMainKatID" type="hidden" size="5" value="(.*?)" />`),
		"nPath":               regexp.MustCompile(`<input name="nPath" id="nPath" disable="false" type="hidden" value="(.*?)" />`),
		"nHide":               regexp.MustCompile(`<input name="nHide" id="nHide" type="hidden" value="(.*?)" />`),
		"nPrekrij":            regexp.MustCompile(`<input style="display:none;" type="hidden" name="nPrekrij" value="(.*?)" />`),
		"nStep":               regexp.MustCompile(`<input style="display:none;" type="hidden" name="nStep" value="(.*?)" />`),
		"lNonJava":            regexp.MustCompile(`<input style="display:none;" type="hidden" name="lNonJava" value="(.*?)" />`),
		"ukaz":                regexp.MustCompile(`<input style="display:none;" type="hidden" name="ukaz" value="(.*?)" />`),
		"bShowForm":           regexp.MustCompile(`<input style="display:none;" type="hidden" name="bShowForm" id=bShowForm value="(.*?)" />`),
		"lEdit":               regexp.MustCompile(`<input style="display:none;" type="hidden" name="lEdit" value="(.*?)" />`),
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	matches := make(map[string]string)
	for k, r := range regex {
		m := r.FindSubmatch(body)
		if m == nil {
			return nil, errors.New("failed to get all meta data")
		}

		matches[k] = string(m[1])
	}

	return matches, nil
}

func (c *Client) publishAd(ad *Ad, metaInfo map[string]string) error {
	buff := &bytes.Buffer{}
	w := multipart.NewWriter(buff)
	defer w.Close()

	// write meta info
	for k, v := range metaInfo {
		err := w.WriteField(k, v)
		if err != nil {
			return err
		}
	}

	// write visible ad fields
	params := map[string]string{
		"cNaziv":     ad.Title,
		"cOpis":      ad.Description,
		"nCenaStart": ad.Price,
		"nKatID":     ad.CategoryId,
		"cTip":       "O",
	}
	for k, v := range params {
		if err := w.WriteField(k, v); err != nil {
			return err
		}
	}

	// upload images
	for _, img := range ad.Images {
		imgId, err := c.uploadImage(&ad.CategoryId, img)
		if err != nil {
			return err
		}

		if err := w.WriteField("images[][id]", imgId); err != nil {
			return err
		}

		if err := w.WriteField("izd_slike_order[]", imgId); err != nil {
			return err
		}
	}

	req, err := http.NewRequest(http.MethodPost, "http://objava-oglasa.bolha.com/oddaj.php", buff)
	if err != nil {
		return err
	}

	overwrite := map[string]string{
		"Host":                      "objava-oglasa.bolha.com",
		"Origin":                    "http://objava-oglasa.bolha.com",
		"Referer":                   fmt.Sprintf("http://objava-oglasa.bolha.com/oddaj.php?katid=%d&days=30", ad.CategoryId),
		"Upgrade-Insecure-Requests": "1",
	}

	headers := getDefaultHeaders(overwrite)

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	req.Header.Set("Content-Type", w.FormDataContentType())

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (c *Client) uploadImage(categoryId *string, imgPath string) (string, error) {
	buff := &bytes.Buffer{}
	w := multipart.NewWriter(buff)

	f, err := os.Open(imgPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="imagename"`)
	h.Set("Content-Type", "image/png")

	part, err := w.CreatePart(h)
	if err != nil {
		return "", err
	}

	if err := png.Encode(part, img); err != nil {
		return "", err
	}

	w.Close()

	req, err := http.NewRequest(http.MethodPost, "http://objava-oglasa.bolha.com/include/imageUploaderProxy.php", buff)
	if err != nil {
		return "", err
	}

	headers := map[string]string{
		"Host":             "objava-oglasa.bolha.com",
		"Connection":       "keep-alive",
		"Pragma":           "no-cache",
		"Cache-Control":    "no-cache",
		"Origin":           "http://objava-oglasa.bolha.com",
		"User-Agent":       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/68.0.3440.106 Safari/537.36",
		"Accept":           "*/*",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          fmt.Sprintf("http://objava-oglasa.bolha.com/oddaj.php?katid=%s&days=30", *categoryId),
		"Accept-Encoding":  "deflate",
		"Accept-Language":  "en-US,en;q=0.9",
	}
	req.Header["MEDIA-ACTION"] = []string{"save-to-mrs"} // special case, TODO: check if it works lowercase
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	r := regexp.MustCompile(regexImageId)
	m := r.FindSubmatch(resBody)
	if len(m) < 2 {
		return "", errors.New("could not extract uploaded image id")
	}

	imgId := string(m[1])
	return imgId, nil
}

func getDefaultHeaders(overwrite map[string]string) map[string]string {
	headers := map[string]string{
		"Accept":          "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding": "identity",
		"Accept-Language": "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":   "max-age=0",
		"Connection":      "keep-alive",
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
	}

	for k, v := range overwrite {
		headers[k] = v
	}

	return headers
}
