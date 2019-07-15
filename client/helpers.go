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
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36"
)

var (
	activeAdsRegex = regexp.MustCompile(`Šifra oglasa: (\d+).*?<span>(\d+)<\/span><a .*?>Skok na vrh<\/a>`)
)

func (c *Client) allowRedirects(allow bool) {
	if allow {
		c.httpClient.CheckRedirect = nil
	} else {
		c.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

func (c *Client) login(u *User) (string, error) {
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
		return "", err
	}

	for k, v := range map[string]string{
		"Accept-Encoding":  "gzip",
		"User-Agent":       userAgent,
		"Content-Type":     "application/x-www-form-urlencoded",
		"Host":             "login.bolha.com",
		"Origin":           "http://www.bolha.com",
		"Referer":          "http://www.bolha.com/",
		"X-Requested-With": "XMLHttpRequest",
		"X-Site":           "http://www.bolha.com/",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("login failed for '%s'", u.Username)
	}

	var sessionId string
	for _, cookie := range res.Cookies() {
		if cookie.Name == "BOLHA_SSID" {
			sessionId = cookie.Value
			break
		}
	}

	if sessionId == "" {
		return "", errors.New("session cookie not found")
	}

	return sessionId, nil
}

func (c *Client) uploadAd(ad *Ad) (int64, error) {
	log.WithField("ad", ad).Info("uploading ad...")

	var wg sync.WaitGroup

	// TODO handle errors

	var metaData map[string]string
	wg.Add(1)
	go func() {
		defer wg.Done()
		mi, _ := c.getAdMetaData(ad)
		metaData = mi
	}()

	imgIds := make([]string, len(ad.Images))
	for i, img := range ad.Images {
		wg.Add(1)
		i1, img1 := i, img
		go func(i int, img io.Reader) {
			defer wg.Done()
			imgId, _ := c.uploadImage(ad.CategoryId, img1)
			imgIds[i1] = imgId
		}(i1, img1)
	}

	wg.Wait()

	return c.publishAd(ad, metaData, imgIds)
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
		"Accept-Encoding":  "gzip",
		"User-Agent":       userAgent,
		"Content-Type":     "application/x-www-form-urlencoded",
		"Host":             "moja.bolha.com",
		"Origin":           "https://moja.bolha.com",
		"Referer":          "https://moja.bolha.com/oglasi",
		"X-Requested-With": "XMLHttpRequest",
	} {
		req.Header.Set(k, v)
	}

	res, err := c.doWithSessionCookie(req)
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
		"Accept-Encoding":  "gzip",
		"User-Agent":       userAgent,
		"Content-Type":     "application/x-www-form-urlencoded",
		"Host":             "moja.bolha.com",
		"Origin":           "https://moja.bolha.com",
		"Referer":          "https://moja.bolha.com/oglasi",
		"X-Requested-With": "XMLHttpRequest",
	} {
		req.Header.Set(k, v)
	}

	// FIXME bolha sends 500 event if ads are removed
	// ignore status code for now, not critical
	_, err = c.httpClient.Do(req)
	return err
}

func (c *Client) getActiveAds() ([]*ActiveAd, error) {
	req, err := http.NewRequest(http.MethodGet, "https://moja.bolha.com/oglasi", nil)
	if err != nil {
		return nil, err
	}

	for k, v := range map[string]string{
		"Accept-Encoding": "gzip",
		"User-Agent":      userAgent,
		"Host":            "moja.bolha.com",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.doWithSessionCookie(req)
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

	m := activeAdsRegex.FindAllSubmatch(body, -1)

	ads := make([]*ActiveAd, len(m))
	for i := range m {
		idI, err := strconv.ParseInt(string(m[i][1]), 10, 64)
		if err != nil {
			return nil, err
		}
		orderI, err := strconv.ParseInt(string(m[i][2]), 10, 32)
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

func (c *Client) getActiveAd(id int64) (*ActiveAd, error) {
	req, err := http.NewRequest(http.MethodGet, "https://moja.bolha.com/oglasi", nil)
	if err != nil {
		return nil, err
	}

	for k, v := range map[string]string{
		"Accept-Encoding": "gzip",
		"User-Agent":      userAgent,
		"Host":            "moja.bolha.com",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.doWithSessionCookie(req)
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

	r, err := regexp.Compile(fmt.Sprintf(`Šifra oglasa: %d.*?<span>(\d+)<\/span><a .*?>Skok na vrh<\/a>`, id))
	if err != nil {
		return nil, err
	}

	m := r.FindSubmatch(body)

	orderI, err := strconv.ParseInt(string(m[1]), 10, 32)
	if err != nil {
		return nil, err
	}

	ad := &ActiveAd{
		Id:    id,
		Order: int(orderI),
	}

	return ad, nil
}

func (c *Client) getAdMetaData(ad *Ad) (map[string]string, error) {
	req, err := http.NewRequest(
		http.MethodPost,
		"http://objava-oglasa.bolha.com/izbor_paketa.php",
		strings.NewReader(
			url.Values{"categoryId": {strconv.Itoa(ad.CategoryId)}}.Encode(),
		),
	)
	if err != nil {
		return nil, err
	}

	for k, v := range map[string]string{
		"Content-Type":    "application/x-www-form-urlencoded",
		"Host":            "objava-oglasa.bolha.com",
		"Origin":          "http://objava-oglasa.bolha.com",
		"Referer":         "http://objava-oglasa.bolha.com/",
		"Accept-Encoding": "gzip",
		"User-Agent":      userAgent,
	} {
		req.Header.Add(k, v)
	}

	c.allowRedirects(true)
	defer func() {
		c.allowRedirects(false)
	}()

	res, err := c.doWithSessionCookie(req)
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

	log.Info("extracted ad meta info")

	return matches, nil
}

func (c *Client) publishAd(ad *Ad, metaData map[string]string, imgIds []string) (int64, error) {
	buff := new(bytes.Buffer)

	mpw := multipart.NewWriter(buff)
	defer mpw.Close()

	// write meta info
	for k, v := range metaData {
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
	for _, imgId := range imgIds {
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
		"Accept-Encoding": "gzip",
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36",
		"Host":            "objava-oglasa.bolha.com",
		"Origin":          "http://objava-oglasa.bolha.com",
		"Referer":         fmt.Sprintf("http://objava-oglasa.bolha.com/oddaj.php?katid=%d&days=30", ad.CategoryId),
		"Content-Type":    mpw.FormDataContentType(),
	} {
		req.Header.Add(k, v)
	}

	res, err := c.doWithSessionCookie(req)
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
	log.Println("uploading image...")

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
		"Accept-Encoding":  "gzip",
		"Host":             "objava-oglasa.bolha.com",
		"Origin":           "http://objava-oglasa.bolha.com",
		"User-Agent":       userAgent,
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          fmt.Sprintf("http://objava-oglasa.bolha.com/oddaj.php?katid=%d&days=30", categoryId),
		"Content-Type":     mpw.FormDataContentType(),

		"MEDIA-ACTION": "save-to-mrs",
	} {
		req.Header.Add(k, v)
	}

	res, err := c.doWithSessionCookie(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error uploading image, status code = %d", res.StatusCode)
	}

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

func (c *Client) doWithSessionCookie(req *http.Request) (*http.Response, error) {
	req.AddCookie(&http.Cookie{
		Name:   "BOLHA_SSID",
		Value:  c.sessionId,
		Path:   "/",
		Domain: ".bolha.com",
	})

	return c.httpClient.Do(req)
}
