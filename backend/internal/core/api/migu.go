package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
)

const (
	MiguDefaultActivity = "MAC_KF_CBH_JBZX"
	miguBaseURL         = "https://app.c.nf.migu.cn"
	miguTicketSource    = "012008"
	miguH5Source        = "220029"
)

// MiguInt accepts both JSON numbers and decimal strings without float rounding.
type MiguInt int64

func (n *MiguInt) UnmarshalJSON(raw []byte) error {
	value := strings.Trim(string(raw), `"`)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return fmt.Errorf("咪咕返回了无效的非负整数")
	}
	*n = MiguInt(parsed)
	return nil
}

type MiguError struct{ Operation, Code, Info string }

func (e *MiguError) Error() string {
	return fmt.Sprintf("咪咕%s失败（%s）：%s", e.Operation, e.Code, e.Info)
}

func MiguErrorCode(err error, code string) bool {
	var business *MiguError
	return errors.As(err, &business) && business.Code == code
}

func MiguSessionExpired(err error) bool {
	var business *MiguError
	if !errors.As(err, &business) {
		return false
	}
	// 200002 means already claimed on SONG rewards; it is not a login signal.
	return strings.Contains(business.Info, "未登录") || strings.Contains(business.Info, "登录失效") ||
		strings.Contains(business.Info, "登录已过期") || strings.Contains(strings.ToLower(business.Info), "token失效")
}

// MiguSession is cached only in account-scoped task storage, never in task logs.
type MiguSession struct {
	Phone           string `json:"phone"`
	UID             string `json:"uid"`
	MSISDN          string `json:"msisdn"`
	PacmToken       string `json:"pacmtoken"`
	DeviceID        string `json:"device_id"`
	IP              string `json:"ip"`
	AuthFingerprint string `json:"auth_fingerprint"`
	CreatedAt       int64  `json:"created_at"`
}

type MiguAPI struct {
	client     *corehttp.Client
	deviceID   string
	ip         string
	activityID string
	session    *MiguSession
	offset     int64
	now        func() time.Time
}

func NewMiguAPI(client *corehttp.Client, deviceID, ip, activityID string) *MiguAPI {
	if activityID == "" {
		activityID = MiguDefaultActivity
	}
	return &MiguAPI{client: client, deviceID: deviceID, ip: ip, activityID: activityID, now: time.Now}
}

func (api *MiguAPI) RestoreSession(session *MiguSession) { api.session = session }

func (api *MiguAPI) Login(authorization, phone string) (*MiguSession, error) {
	if strings.TrimSpace(authorization) == "" || strings.TrimSpace(phone) == "" {
		return nil, fmt.Errorf("咪咕换票缺少云盘账号凭据")
	}
	api.session = nil
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ticket, err := auth.NewAuth(api.client).QuerySpecTokenForSourceContext(ctx, authorization, phone, miguTicketSource)
	if err != nil {
		return nil, fmt.Errorf("咪咕云盘出票失败: %w", err)
	}
	var data struct {
		UserID    json.RawMessage `json:"userId"`
		UID       json.RawMessage `json:"uid"`
		MSISDN    string          `json:"msisdn"`
		PacmToken string          `json:"pacmtoken"`
	}
	headers, err := api.request("GET", "/user/h5/token-validate/v3.0", url.Values{
		"token": {"ANDT" + ticket}, "type": {"2"}, "sourceId": {miguH5Source},
	}, nil, &data)
	if err != nil {
		return nil, err
	}
	uid := strings.Trim(string(data.UserID), `"`)
	if uid == "" || uid == "null" {
		uid = strings.Trim(string(data.UID), `"`)
	}
	if uid == "" || uid == "null" {
		return nil, fmt.Errorf("咪咕验票未返回用户标识")
	}
	pacmToken := headers.Get("pacmtoken")
	if pacmToken == "" {
		pacmToken = data.PacmToken
	}
	if pacmToken == "" {
		return nil, fmt.Errorf("咪咕验票未返回会话凭证")
	}
	if len(data.MSISDN) == 11 && !strings.Contains(data.MSISDN, "*") && data.MSISDN != phone {
		return nil, fmt.Errorf("咪咕验票账号与当前云盘账号不一致")
	}
	if data.MSISDN == "" {
		data.MSISDN = phone
	}
	session := &MiguSession{Phone: phone, UID: uid, MSISDN: data.MSISDN, PacmToken: pacmToken,
		DeviceID: api.deviceID, IP: api.ip, CreatedAt: api.now().Unix()}
	api.session = session
	return session, nil
}

func (api *MiguAPI) ce() string {
	if api.session == nil {
		return miguEncode("ip=" + api.ip + "&deviceId=" + api.deviceID)
	}
	// The original account phone is required here, not token-validate's masked number.
	return miguEncode("uid=" + api.session.UID + "&ip=" + api.ip + "&msisdn=" + api.session.Phone + "&deviceId=" + api.deviceID)
}

func (api *MiguAPI) request(method, path string, query url.Values, body []byte, out interface{}) (http.Header, error) {
	signURL := path
	if len(query) > 0 {
		signURL += "?" + query.Encode()
	}
	ts := strconv.FormatInt(api.now().UnixMilli()+api.offset, 10)
	ce := api.ce()
	headers := map[string]string{
		"channel": "0146921", "ua": "Android_migu", "mode": "android", "appid": "music",
		"brand": "Redmi", "version": "8.9.6", "language": "Chinese", "os": "Android",
		"osversion": "Android 13", "platform": "23049RAD8C", "pkgname": "cmccwm.mobilemusic",
		"buildid": "20261029174323", "recommendstatus": "0", "subchannel": "", "hwid": "", "oaid": "", "gsm": "",
		"randomsessionkey": "000000", "verify": "verify", "mgm-network-type": "04",
		"mgm-network-operators": "04", "mgm-network-standard": "01", "mgm-user-agent": "23049RAD8C",
		"logid": ts, "ce": ce, "token": "", "timestamp": ts, "signversion": "V005", "deviceId": api.deviceID,
		"activityId": api.activityID, "Accept": "application/json", "Accept-Language": "zh-CN,zh;q=0.9",
		"User-Agent": "Mozilla/5.0 (Linux; U; Android 13; zh-cn; 23049RAD8C Build/TKQ1.221114.001) AppleWebKit/533.1 Mobile Safari/533.1",
	}
	if api.session != nil {
		headers["pacmtoken"], headers["uid"], headers["msisdn"] = api.session.PacmToken, api.session.UID, api.session.MSISDN
	}
	if body != nil {
		headers["Content-Type"] = "application/json;charset=utf-8"
	}
	input := append([]byte(signURL+ce+ts), body...)
	headers["sign"] = miguSign(input, miguCertificatePrefix)
	// GET includes single-use ticket consumption and state-changing activity
	// operations. Never replay these, inherit cloud headers, or follow redirects.
	resp, err := api.client.RequestWithOptions(method, miguBaseURL+signURL, headers, body,
		corehttp.WithoutDefaultHeaders(), corehttp.WithoutCookieJar(), corehttp.WithoutRetries(),
		corehttp.WithoutRedirects(), corehttp.WithRequestTimeout(20*time.Second))
	if err != nil {
		// url.Error includes the single-use ticket query. Keep it out of persisted logs.
		return nil, fmt.Errorf("咪咕%s请求失败，请稍后重查状态", path)
	}
	raw, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("咪咕%s读取响应失败", path)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("咪咕%s返回HTTP %d", path, resp.StatusCode)
	}
	if serverTime, parseErr := strconv.ParseInt(resp.Header.Get("timestamp"), 10, 64); parseErr == nil && serverTime > 0 {
		api.offset = serverTime - api.now().UnixMilli()
	}
	var envelope struct {
		Code string          `json:"code"`
		Info string          `json:"info"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil || envelope.Code == "" {
		return nil, fmt.Errorf("咪咕%s响应格式无效", path)
	}
	if envelope.Code != "000000" {
		info := envelope.Info
		for _, values := range query {
			for _, value := range values {
				if len(value) > 16 {
					info = strings.ReplaceAll(info, value, "[已隐藏]")
				}
			}
		}
		if api.session != nil {
			info = strings.ReplaceAll(info, api.session.PacmToken, "[已隐藏]")
		}
		return nil, &MiguError{Operation: path, Code: envelope.Code, Info: info}
	}
	if out != nil {
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, out) != nil {
			return nil, fmt.Errorf("咪咕%s缺少有效业务数据", path)
		}
	}
	return resp.Header, nil
}

func (api *MiguAPI) JoinActivity() error {
	_, err := api.request("GET", "/act-task/join-activity", nil, nil, nil)
	return err
}

func (api *MiguAPI) UserInfo() error {
	var data map[string]json.RawMessage
	_, err := api.request("GET", "/user/h5/user-info/v1.0", nil, nil, &data)
	return err
}

type MiguTaskInfo struct {
	ID       string          `json:"id"`
	Name     string          `json:"taskName"`
	Complete *MiguInt        `json:"completeTime"`
	Required *MiguInt        `json:"needCompleteTime"`
	Received *MiguInt        `json:"rceiveTime"`
	ExtData  json.RawMessage `json:"extData"`
}

func (api *MiguAPI) QueryTasks() ([]MiguTaskInfo, error) {
	var tasks []MiguTaskInfo
	_, err := api.request("GET", "/act-task/query-tasks", nil, nil, &tasks)
	return tasks, err
}

func (api *MiguAPI) LoginAccess() error {
	_, err := api.request("GET", "/act-task/login-access", nil, nil, nil)
	return err
}

func (api *MiguAPI) ReportTask(taskID string) error {
	body, _ := json.Marshal(map[string]string{"taskId": taskID})
	_, err := api.request("POST", "/act-task/task-report", nil, body, nil)
	return err
}

type MiguSignStatus struct {
	SignTask   *bool   `json:"signTask"`
	IsSign     *bool   `json:"isSign"`
	SignNum    MiguInt `json:"signNum"`
	CurrentDay MiguInt `json:"currentDay"`
	Days       []struct {
		Day           MiguInt  `json:"day"`
		ReceiveStatus *MiguInt `json:"receiveStatus"`
	} `json:"days"`
}

func (api *MiguAPI) SignStatus(newUser bool) (*MiguSignStatus, error) {
	path := "/act-jbzx/sign/status"
	if newUser {
		path = "/act-jbzx/new-user-receive/status"
	}
	var status MiguSignStatus
	_, err := api.request("GET", path, nil, nil, &status)
	return &status, err
}

func (api *MiguAPI) Sign(newUser bool) error {
	path, body := "/act-jbzx/sign", []byte(nil)
	if newUser {
		path, body = "/act-jbzx/new-user-receive", []byte("{}")
	}
	_, err := api.request("POST", path, nil, body, nil)
	return err
}

type MiguSongStatus struct {
	SongTime     *MiguInt `json:"songTime"`
	AllCompleted bool     `json:"allCompleted"`
	Tasks        []struct {
		ID     string   `json:"id"`
		Status *MiguInt `json:"status"`
	} `json:"tasks"`
}

func (api *MiguAPI) SongStatus() (*MiguSongStatus, error) {
	var status MiguSongStatus
	_, err := api.request("GET", "/act-jbzx/song-task/v2.0", nil, nil, &status)
	return &status, err
}

func (api *MiguAPI) ReportSong(duration int64, transactionID string) error {
	if duration <= 0 || duration > 30*60*1000 || transactionID == "" {
		return fmt.Errorf("咪咕听歌上报参数无效")
	}
	body, _ := json.Marshal(struct {
		Duration      int64  `json:"duration"`
		TransactionID string `json:"transactionId"`
	}{duration, transactionID})
	_, err := api.request("POST", "/act-message/report/song-duration/v1", nil, []byte(miguEncode(string(body))), nil)
	return err
}

func (api *MiguAPI) ClaimSong(taskID string) (int64, error) {
	body, _ := json.Marshal(map[string]string{"taskId": taskID})
	var reward struct {
		Amount *MiguInt `json:"aiBeanNum"`
	}
	_, err := api.request("POST", "/act-jbzx/receive-task-rewards/v2.0", nil, body, &reward)
	if err == nil && reward.Amount != nil {
		return int64(*reward.Amount), nil
	}
	return 0, err
}

// Assets returns an unread reward-notice count and the current AI-note balance.
// music-bean-accumulate consumes the notice count: it is neither a wallet
// balance nor lifetime earnings, and its changes must never be booked as gains.
func (api *MiguAPI) Assets() (int64, int64, error) {
	var notice struct {
		Num *MiguInt `json:"num"`
	}
	_, noticeErr := api.request("GET", "/act-jbzx/music-bean-accumulate", nil, nil, &notice)
	notes, noteErr := api.NoteBalance()
	if err := errors.Join(noticeErr, noteErr); err != nil {
		return 0, 0, err
	}
	if notice.Num == nil {
		return 0, 0, fmt.Errorf("咪咕查询缺少奖励提示数")
	}
	return int64(*notice.Num), notes, nil
}

func (api *MiguAPI) NoteBalance() (int64, error) {
	var notes struct {
		Num *MiguInt `json:"availableNum"`
	}
	_, err := api.request("GET", "/act-jbzx/ai-bean/query-available", nil, nil, &notes)
	if err != nil {
		return 0, err
	}
	if notes.Num == nil {
		return 0, fmt.Errorf("咪咕查询缺少音符余额")
	}
	return int64(*notes.Num), nil
}

func (api *MiguAPI) MusicBeanFlow(page, size int) (json.RawMessage, error) {
	if page < 1 || size < 1 || size > 100 {
		return nil, fmt.Errorf("咪咕金币流水分页参数无效")
	}
	var data json.RawMessage
	_, err := api.request("GET", "/act-jbzx/music-bean-flow", url.Values{
		"pageNumber": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)},
	}, nil, &data)
	return data, err
}
