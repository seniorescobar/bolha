package client

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

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

func (c *Client) login(u *User) error {
	req, err := http.NewRequest(
		http.MethodPost,
		"https://login.bolha.com/auth.php",
		strings.NewReader(
			url.Values{
				"username":   {u.Username},
				"password":   {u.Password},
				"rememberMe": {"true"},
			}.Encode(),
		),
	)
	if err != nil {
		return err
	}

	for k, v := range map[string]string{
		"Accept":                    "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding":           "identity",
		"Accept-Language":           "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":             "max-age=0",
		"Connection":                "keep-alive",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "login.bolha.com",
		"Origin":                    "http://www.bolha.com",
		"Referer":                   "http://www.bolha.com/",
		"Upgrade-Insecure-Requests": "1",
		"X-Requested-With":          "XMLHttpRequest",
		"X-Site":                    "http://www.bolha.com/",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("login failed for '%s'", u.Username)
	}

	return nil
}

func (c *Client) uploadAd(ad *Ad) (int64, error) {
	log.WithField("ad", ad).Info("uploading ad...")

	metaInfo, err := c.getAdMetaInfo(ad)
	if err != nil {
		return 0, err
	}

	return c.publishAd(ad, metaInfo)
}

func (c *Client) removeAd(id int64) error {
	log.WithField("id", id).Info("removing ad...")

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("https://moja.bolha.com/adManager/ajaxRemoveActive/id/%d", id),
		nil,
	)
	if err != nil {
		return err
	}

	for k, v := range map[string]string{
		"Accept":                    "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding":           "gzip",
		"Accept-Language":           "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":             "max-age=0",
		"Connection":                "keep-alive",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "moja.bolha.com",
		"Origin":                    "https://moja.bolha.com",
		"Referer":                   "https://moja.bolha.com/oglasi",
		"Upgrade-Insecure-Requests": "1",
		"X-Requested-With":          "XMLHttpRequest",
	} {
		req.Header.Set(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		fmt.Errorf("erroring removing ad (StatusCode=%d)", res.StatusCode)
	}

	return err
}

func (c *Client) removeAds(ids []int64) error {
	log.WithField("ids", ids).Info("removing ads...")

	sIds := make([]string, len(ids))
	for i, id := range ids {
		sIds[i] = strconv.FormatInt(id, 10)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://moja.bolha.com/adManager/ajaxRemoveActiveBulk",
		strings.NewReader(
			url.Values{
				"IDS": {strings.Join(sIds, ",")},
			}.Encode(),
		),
	)
	if err != nil {
		return err
	}

	for k, v := range map[string]string{
		"Accept":                    "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding":           "gzip",
		"Accept-Language":           "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":             "max-age=0",
		"Connection":                "keep-alive",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "moja.bolha.com",
		"Origin":                    "https://moja.bolha.com",
		"Referer":                   "https://moja.bolha.com/oglasi",
		"Upgrade-Insecure-Requests": "1",
		"X-Requested-With":          "XMLHttpRequest",
	} {
		req.Header.Set(k, v)
	}

	// TODO check resp status code
	_, err = c.httpClient.Do(req)
	return err
}

func (c *Client) getActiveAds() ([]*ActiveAd, error) {
	req, err := http.NewRequest(http.MethodGet, "https://moja.bolha.com/oglasi", nil)
	if err != nil {
		return nil, err
	}

	for k, v := range map[string]string{
		"Accept":                    "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding":           "gzip",
		"Accept-Language":           "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":             "max-age=0",
		"Connection":                "keep-alive",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
		"Host":                      "moja.bolha.com",
		"Upgrade-Insecure-Requests": "1",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	gzReader, err := gzip.NewReader(res.Body)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	body, err := ioutil.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	var (
		mId    = regexp.MustCompile(`Šifra oglasa: (\d+)`).FindAllStringSubmatch(string(body), -1)
		mOrder = regexp.MustCompile(`<span>(\d+)<\/span><a .*>Skok na vrh<\/a>`).FindAllStringSubmatch(string(body), -1)
	)

	if len(mId) != len(mOrder) {
		return nil, errors.New("regex matches len does not match")
	}

	ads := make([]*ActiveAd, len(mId))
	for i := range mId {
		idI, err := strconv.ParseInt(mId[i][1], 10, 64)
		if err != nil {
			return nil, err
		}
		orderI, err := strconv.ParseInt(mOrder[i][1], 10, 32)
		if err != nil {
			return nil, err
		}

		ads[i] = &ActiveAd{
			Id:    idI,
			Order: int(orderI),
		}
	}

	return ads, nil
}

func (c *Client) getAdMetaInfo(ad *Ad) (map[string]string, error) {
	values := url.Values{
		"categoryId": {
			strconv.Itoa(ad.CategoryId),
		},
	}

	req, err := http.NewRequest(http.MethodPost, "http://objava-oglasa.bolha.com/izbor_paketa.php", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}

	for k, v := range map[string]string{
		"Content-Type":              "application/x-www-form-urlencoded",
		"Host":                      "objava-oglasa.bolha.com",
		"Origin":                    "http://objava-oglasa.bolha.com",
		"Referer":                   "http://objava-oglasa.bolha.com/",
		"Upgrade-Insecure-Requests": "1",
		"Accept":                    "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding":           "gzip",
		"Accept-Language":           "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":             "max-age=0",
		"Connection":                "keep-alive",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
	} {
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

	gzReader, err := gzip.NewReader(res.Body)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

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

	body, err := ioutil.ReadAll(gzReader)
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

func (c *Client) publishAd(ad *Ad, metaInfo map[string]string) (int64, error) {
	buff := new(bytes.Buffer)

	mpw := multipart.NewWriter(buff)
	defer mpw.Close()

	// write meta info
	for k, v := range metaInfo {
		err := mpw.WriteField(k, v)
		if err != nil {
			return 0, err
		}
	}

	// write visible ad fields
	for k, v := range map[string]string{
		"cNaziv":     ad.Title,
		"cOpis":      ad.Description,
		"nCenaStart": strconv.Itoa(ad.Price),
		"nKatID":     strconv.Itoa(ad.CategoryId),
		"cTip":       "O",
	} {
		if err := mpw.WriteField(k, v); err != nil {
			return 0, err
		}
	}

	// upload images
	for _, img := range ad.Images {
		imgId, err := c.uploadImage(ad.CategoryId, img)
		if err != nil {
			return 0, err
		}

		if err := mpw.WriteField("images[][id]", imgId); err != nil {
			return 0, err
		}

		if err := mpw.WriteField("izd_slike_order[]", imgId); err != nil {
			return 0, err
		}
	}

	req, err := http.NewRequest(http.MethodPost, "http://objava-oglasa.bolha.com/oddaj.php", buff)
	if err != nil {
		return 0, err
	}

	for k, v := range map[string]string{
		"Accept":                    "application/json, text/javascript, */*; q=0.01",
		"Accept-Encoding":           "gzip",
		"Accept-Language":           "en-US,en;q=0.9,sl;q=0.8,hr;q=0.7",
		"Cache-Control":             "max-age=0",
		"Connection":                "keep-alive",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
		"Host":                      "objava-oglasa.bolha.com",
		"Origin":                    "http://objava-oglasa.bolha.com",
		"Referer":                   fmt.Sprintf("http://objava-oglasa.bolha.com/oddaj.php?katid=%d&days=30", ad.CategoryId),
		"Upgrade-Insecure-Requests": "1",
		"Content-Type":              mpw.FormDataContentType(),
	} {
		req.Header.Add(k, v)
	}

	c.allowRedirects(false)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	res.Body.Close()

	if res.StatusCode != http.StatusFound {
		return 0, errors.New("error publishing ad")
	}

	// perf hack
	// different approach would be to call res.Location() and parse query params
	loc := res.Header.Get("Location")
	id, err := strconv.ParseInt(loc[20:30], 10, 64)
	if err != nil {
		return 0, err
	}
	log.WithField("id", id).Info("extracted uploaded ad id")

	return id, nil
}

func (c *Client) uploadImage(categoryId int, img io.Reader) (string, error) {
	buff := new(bytes.Buffer)

	mpw := multipart.NewWriter(buff)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="imagename"`)
	h.Set("Content-Type", "image/png")

	part, err := mpw.CreatePart(h)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(part, img); err != nil {
		return "", err
	}

	if err := mpw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, "http://objava-oglasa.bolha.com/include/imageUploaderProxy.php", buff)
	if err != nil {
		return "", err
	}

	for k, v := range map[string]string{
		"Host":             "objava-oglasa.bolha.com",
		"Connection":       "keep-alive",
		"Pragma":           "no-cache",
		"Cache-Control":    "no-cache",
		"Origin":           "http://objava-oglasa.bolha.com",
		"User-Agent":       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/68.0.3440.106 Safari/537.36",
		"Accept":           "*/*",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          fmt.Sprintf("http://objava-oglasa.bolha.com/oddaj.php?katid=%d&days=30", categoryId),
		"Accept-Encoding":  "gzip",
		"Accept-Language":  "en-US,en;q=0.9",
		"Content-Type":     mpw.FormDataContentType(),

		"MEDIA-ACTION": "save-to-mrs",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	gzr, err := gzip.NewReader(res.Body)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	// perf hack, uuid is 35 chars long
	id := make([]byte, 36)
	gzr.Read(make([]byte, 15))
	gzr.Read(id)

	idStr := string(id)

	if _, err := uuid.Parse(idStr); err != nil {
		return "", fmt.Errorf(`invalid uuid %s`, idStr)
	}

	log.WithField("id", idStr).Info("extracted uploaded image id")

	return idStr, nil
}
