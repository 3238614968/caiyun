package tasks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"

	"github.com/google/uuid"
)

// MiguTask shares one independent music session across this account's batch.
// The cloud client's auth, JWT, cookies and device identity are never changed.
type MiguTask struct {
	accountTaskSettings
	client      *corehttp.Client
	api         *api.MiguAPI
	storage     Storage
	auth        string
	initialized bool
	initErr     error
	pending     bool
	skipped     bool
	message     string
	notesGained int
	now         func() time.Time
}

func NewMiguTask(client *corehttp.Client) *MiguTask {
	return &MiguTask{client: client, now: activityNow}
}

func (t *MiguTask) SetAccountContext(phone, authorization string) *MiguTask {
	t.SetAccountPhone(phone)
	t.auth = strings.TrimSpace(authorization)
	return t
}

func (t *MiguTask) SetStorage(storage Storage) *MiguTask { t.storage = storage; return t }
func (t *MiguTask) Pending() bool                        { return t.pending }
func (t *MiguTask) Skipped() bool                        { return t.skipped }
func (t *MiguTask) Message() string                      { return t.message }
func (t *MiguTask) NotesGained() int                     { return t.notesGained }

// Read the wallet around a mutation without consuming reward notifications.
// A missing snapshot cannot turn the existing balance into today's earnings.
func (t *MiguTask) trackNoteBalance() func() {
	before, beforeErr := t.api.NoteBalance()
	return func() {
		after, afterErr := t.api.NoteBalance()
		if beforeErr == nil && afterErr == nil && before >= 0 && after > before {
			t.notesGained = int(after - before)
			t.message += fmt.Sprintf("，本次AI音符%d", t.notesGained)
		}
	}
}

func (t *MiguTask) sessionKey() string {
	sum := sha256.Sum256([]byte(t.settingsPhone))
	return "migu:session:" + hex.EncodeToString(sum[:16])
}

func (t *MiguTask) ensureSession() error {
	if t.initialized {
		return t.initErr
	}
	t.initialized = true
	t.initErr = t.openSession()
	return t.initErr
}

func (t *MiguTask) openSession() error {
	if t.client == nil || t.settingsPhone == "" || t.auth == "" {
		return fmt.Errorf("咪咕活动缺少云盘账号凭据")
	}
	deviceID := t.setting("CAIYUN_MIGU_DEVICE_ID")
	if deviceID == "" {
		// Stable across workers and token refreshes, different for each account.
		sum := sha256.Sum256([]byte("caiyun:migu:android:" + t.settingsPhone))
		deviceID = hex.EncodeToString(sum[:8])
	}
	if raw, err := hex.DecodeString(deviceID); err != nil || len(raw) != 8 {
		return fmt.Errorf("咪咕设备标识须为此账号的16位十六进制Android ID")
	}
	ip := t.setting("CAIYUN_MIGU_IP")
	if ip == "" {
		ip = "127.0.0.1"
	}
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("咪咕IP配置无效")
	}
	activityID := strings.TrimSpace(os.Getenv("CAIYUN_MIGU_ACTIVITY_ID"))
	t.api = api.NewMiguAPI(t.client, deviceID, ip, activityID)
	credential := t.auth
	if len(credential) >= 6 && strings.EqualFold(credential[:6], "Basic ") {
		credential = strings.TrimSpace(credential[6:])
	}
	fingerprint := sha256.Sum256([]byte(credential))
	authFingerprint := hex.EncodeToString(fingerprint[:])
	if t.storage != nil {
		if cached, err := t.storage.Get(t.sessionKey()); err == nil {
			var session api.MiguSession
			if json.Unmarshal([]byte(cached), &session) == nil && session.Phone == t.settingsPhone &&
				session.DeviceID == deviceID && session.IP == ip && session.AuthFingerprint == authFingerprint &&
				session.UID != "" && session.PacmToken != "" && session.CreatedAt > 0 &&
				t.now().Unix() >= session.CreatedAt && t.now().Unix()-session.CreatedAt < 600 {
				t.api.RestoreSession(&session)
				// Probe the reusable H5 credential, never re-consume an SSO ticket.
				if err := t.api.UserInfo(); err == nil {
					return t.api.JoinActivity()
				} else if !api.MiguSessionExpired(err) {
					return err
				}
			}
		}
		// A changed/expired credential must not survive a failed new exchange.
		if err := t.storage.Delete(t.sessionKey()); err != nil {
			return fmt.Errorf("咪咕旧会话缓存清理失败")
		}
	}
	session, err := t.api.Login(t.auth, t.settingsPhone)
	if err != nil {
		return err
	}
	session.AuthFingerprint = authFingerprint
	if t.storage != nil {
		raw, _ := json.Marshal(session)
		if err := t.storage.Set(t.sessionKey(), string(raw)); err != nil {
			return fmt.Errorf("咪咕会话缓存保存失败")
		}
	}
	return t.api.JoinActivity()
}

func (t *MiguTask) Run(code string) error {
	t.pending, t.skipped, t.message = false, false, ""
	t.notesGained = 0
	switch code {
	case "migu_login", "migu_signin", "migu_new_user", "migu_community", "migu_mobile", "migu_noon", "migu_night", "migu_song", "migu_assets":
	default:
		return fmt.Errorf("未知咪咕任务: %s", code)
	}
	if err := t.ensureSession(); err != nil {
		return err
	}
	var err error
	switch code {
	case "migu_login":
		err = t.loginGift()
	case "migu_signin", "migu_new_user":
		err = t.sign(code == "migu_new_user")
	case "migu_song":
		err = t.songs()
	case "migu_assets":
		var notice, notes int64
		notice, notes, err = t.api.Assets()
		if err == nil {
			t.message = fmt.Sprintf("咪咕AI音符余额%d，本次奖励提示%d（非累计余额）", notes, notice)
		}
	default:
		id := map[string]string{"migu_community": "qsq", "migu_mobile": "zyd-sign", "migu_noon": "noon-check", "migu_night": "night-check"}[code]
		err = t.report(id)
	}
	if api.MiguSessionExpired(err) {
		if t.storage != nil {
			_ = t.storage.Delete(t.sessionKey())
		}
		// Do not rerun a partially executed mutation sequence to renew auth.
		t.initErr = fmt.Errorf("咪咕会话已失效，下次执行重新换票: %w", err)
	}
	return err
}

func miguTaskDone(task api.MiguTaskInfo) (bool, error) {
	if task.Complete == nil {
		return false, fmt.Errorf("咪咕任务%s缺少完成次数", task.ID)
	}
	required := api.MiguInt(1)
	if task.Required != nil && *task.Required > 0 {
		required = *task.Required
	}
	return *task.Complete >= required, nil
}

func (t *MiguTask) findTask(match func(api.MiguTaskInfo) bool) (*api.MiguTaskInfo, error) {
	items, err := t.api.QueryTasks()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if match(items[i]) {
			return &items[i], nil
		}
	}
	return nil, nil
}

func (t *MiguTask) loginGift() error {
	match := func(task api.MiguTaskInfo) bool {
		return task.ID == "xrrw" || strings.Contains(task.Name, "登录即送")
	}
	task, err := t.findTask(match)
	if err != nil {
		return err
	}
	if task == nil {
		t.pending, t.message = true, "当前活动未返回登录即送任务"
		return nil
	}
	done, err := miguTaskDone(*task)
	if err != nil {
		return err
	}
	if done {
		// Behavior rewards settle automatically. rceiveTime is a presentation
		// field, not a separate claim prerequisite (SONG uses its own receipts).
		t.message = "咪咕登录即送已完成，奖励由活动自动结算"
		return nil
	}
	defer t.trackNoteBalance()()
	submitErr := t.api.LoginAccess()
	after, checkErr := t.findTask(match)
	if checkErr != nil {
		return errors.Join(submitErr, checkErr)
	}
	if after != nil {
		if confirmed, err := miguTaskDone(*after); err != nil {
			return errors.Join(submitErr, err)
		} else if confirmed {
			t.message = "咪咕登录即送完成状态已确认，奖励由活动自动结算"
			return nil
		}
	}
	t.pending, t.message = true, "咪咕登录接入已提交，奖励状态待确认（未重复提交）"
	return submitErr
}

func (t *MiguTask) sign(newUser bool) error {
	label := "咪咕每日签到"
	if newUser {
		label = "咪咕新人礼"
	}
	var before *api.MiguSignStatus
	if !newUser {
		// The official page selects exactly one sign-in route with signTask.
		// Calling /sign during the seven-day newcomer route returns 200013,
		// even while the unrelated ordinary sign status says isSign=false.
		route, err := t.api.SignStatus(true)
		if err != nil && !api.MiguErrorCode(err, "200013") {
			return err
		}
		if err == nil && route.SignTask != nil && !*route.SignTask {
			newUser, before = true, route
			label += "（新人礼通道）"
		}
	}
	var err error
	if before == nil {
		before, err = t.api.SignStatus(newUser)
	}
	if newUser && api.MiguErrorCode(err, "200013") {
		t.skipped, t.message = true, "当前账号不符合咪咕新人礼资格"
		return nil
	}
	if err != nil {
		return err
	}
	// Official queryNewUserReceiveStatus/receiveNewUser7Days uses signTask=true
	// for the ordinary sign-in route. That valid response omits isSign/days.
	if newUser && before.SignTask != nil && *before.SignTask {
		t.skipped, t.message = true, "当前账号使用咪咕常规签到，不适用七日新人礼"
		return nil
	}
	done, err := miguSignDone(before, newUser)
	if err != nil {
		return err
	}
	if done {
		t.message = label + "今日已完成"
		return nil
	}
	defer t.trackNoteBalance()()
	submitErr := t.api.Sign(newUser)
	if newUser && api.MiguErrorCode(submitErr, "200013") {
		t.skipped, t.message = true, "当前账号不符合咪咕新人礼资格"
		return nil
	}
	after, checkErr := t.api.SignStatus(newUser)
	if checkErr != nil {
		return errors.Join(submitErr, checkErr)
	}
	confirmed, checkErr := miguSignDone(after, newUser)
	if checkErr != nil {
		return errors.Join(submitErr, checkErr)
	}
	if confirmed {
		t.message = label + "完成状态已确认"
		return nil
	}
	t.pending, t.message = true, label+"已提交，完成状态待确认（未重复提交）"
	return submitErr
}

func miguSignDone(status *api.MiguSignStatus, newUser bool) (bool, error) {
	if status.IsSign != nil {
		return *status.IsSign, nil
	}
	if newUser && status.CurrentDay > 0 {
		for _, day := range status.Days {
			if day.Day == status.CurrentDay && day.ReceiveStatus != nil {
				return *day.ReceiveStatus == 1, nil
			}
		}
	}
	return false, fmt.Errorf("咪咕签到查询缺少今日完成状态")
}

func (t *MiguTask) report(id string) error {
	match := func(task api.MiguTaskInfo) bool { return task.ID == id }
	task, err := t.findTask(match)
	if err != nil {
		return err
	}
	if task == nil {
		t.pending, t.message = true, "当前咪咕活动未提供此任务"
		return nil
	}
	done, err := miguTaskDone(*task)
	if err != nil {
		return err
	}
	if done {
		t.message = task.Name + "已完成"
		return nil
	}
	hour := t.now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Hour()
	if (id == "noon-check" && hour != 12) || (id == "night-check" && hour != 22) {
		window := "12:00–13:00"
		if id == "night-check" {
			window = "22:00–23:00"
		}
		t.pending, t.message = true, "咪咕打卡待北京时间"+window+"内执行"
		return nil
	}
	confirmNotes := t.trackNoteBalance()
	submitErr := t.api.ReportTask(id)
	var refusal *api.MiguError
	if errors.As(submitErr, &refusal) && refusal.Code == "200013" && strings.Contains(refusal.Info, "不在任务时间范围") {
		// Some activity tasks remain visible outside their server-defined
		// dates. Do not guess that window or book earnings for a rejected action.
		t.pending, t.message = true, task.Name+"当前不在服务端开放时间，待开放后执行（200013，本次未完成）"
		return nil
	}
	defer confirmNotes()
	after, checkErr := t.findTask(match)
	if checkErr != nil {
		return errors.Join(submitErr, checkErr)
	}
	if after != nil {
		if confirmed, err := miguTaskDone(*after); err != nil {
			return errors.Join(submitErr, err)
		} else if confirmed {
			t.message = task.Name + "服务端完成状态已确认"
			return nil
		}
	}
	t.pending, t.message = true, task.Name+"已提交，完成状态待确认（未重复提交）"
	return submitErr
}

func miguSongValid(status *api.MiguSongStatus) error {
	if status.SongTime == nil || len(status.Tasks) == 0 {
		return fmt.Errorf("咪咕听歌查询缺少时长或奖励档位")
	}
	seen := map[string]bool{}
	for _, task := range status.Tasks {
		if task.ID == "" || task.Status == nil || *task.Status > 2 || seen[task.ID] {
			return fmt.Errorf("咪咕听歌奖励档位状态无效")
		}
		seen[task.ID] = true
	}
	return nil
}

func (t *MiguTask) songs() error {
	minutes := 30
	if value := strings.TrimSpace(os.Getenv("CAIYUN_MIGU_SONG_MINUTES")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > 30 {
			return fmt.Errorf("咪咕听歌目标分钟须为0到30，0仅领取已达标奖励")
		}
		minutes = parsed
	}
	status, err := t.api.SongStatus()
	if err != nil {
		return err
	}
	if err := miguSongValid(status); err != nil {
		return err
	}
	target := int64(minutes * 60 * 1000)
	allClaimed := true
	for _, task := range status.Tasks {
		allClaimed = allClaimed && *task.Status == 2
	}
	if !allClaimed && int64(*status.SongTime) < target {
		submitErr := t.api.ReportSong(target-int64(*status.SongTime), uuid.NewString())
		status, err = t.api.SongStatus()
		if err != nil {
			return errors.Join(submitErr, err)
		}
		if err := miguSongValid(status); err != nil {
			return errors.Join(submitErr, err)
		}
		if int64(*status.SongTime) < target {
			t.pending, t.message = true, "咪咕听歌时长已提交，服务端进度待确认（未重复上报）"
			return submitErr
		}
	}
	var notes int64
	claimErrors := map[string]error{}
	claimAmounts := map[string]int64{}
	claimed := map[string]bool{}
	for _, task := range status.Tasks {
		// Only the documented SONG family and explicitly claimable tiers.
		if !strings.HasPrefix(task.ID, "SONG_") || *task.Status != 1 {
			continue
		}
		amount, err := t.api.ClaimSong(task.ID)
		if err != nil && !api.MiguErrorCode(err, "200002") {
			claimErrors[task.ID] = err
		}
		claimAmounts[task.ID] = amount
		claimed[task.ID] = true
	}
	if len(claimed) > 0 {
		after, err := t.api.SongStatus()
		if err != nil {
			return err
		}
		if err := miguSongValid(after); err != nil {
			return err
		}
		for _, task := range after.Tasks {
			if *task.Status == 2 && claimAmounts[task.ID] > 0 {
				notes += claimAmounts[task.ID]
			}
			if *task.Status == 2 {
				delete(claimed, task.ID)
				delete(claimErrors, task.ID)
			}
		}
		status = after
	}
	t.notesGained = int(notes)
	confirmed := 0
	// Thresholds documented in the reference contract §9.3, in milliseconds.
	thresholds := map[string]int64{"SONG_1": 30000, "SONG_2": 180000, "SONG_3": 300000, "SONG_4": 600000, "SONG_5": 900000, "SONG_6": 1200000, "SONG_7": 1800000}
	for _, task := range status.Tasks {
		if *task.Status == 2 {
			confirmed++
		} else if *task.Status == 1 || (minutes > 0 && thresholds[task.ID] > 0 && int64(*status.SongTime) >= thresholds[task.ID]) {
			t.pending = true
		}
	}
	t.pending = t.pending || len(claimed) > 0
	t.message = fmt.Sprintf("咪咕听歌%.1f分钟，已确认领取%d/%d档，本次AI音符%d", float64(*status.SongTime)/60000, confirmed, len(status.Tasks), notes)
	if t.pending {
		t.message += "，其余档位待确认（未重复领取）"
	}
	var issues []error
	for _, err := range claimErrors {
		issues = append(issues, err)
	}
	return errors.Join(issues...)
}
