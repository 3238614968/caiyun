package tasks

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// MailSendDedupStore is shared by workers so one sender-recipient pair is
// claimed only once per calendar month, even when workers run concurrently.
type MailSendDedupStore interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	DelIfValue(key, value string) (bool, error)
}

type MailPeer struct {
	AccountID uint
	Phone     string
}

type mail139Sender interface {
	Send139Mail(session *api.Mail139Session, senderPhone, recipientEmail, subject, content string) error
}

type MailMutualTask struct {
	api       *api.CaiyunAPI
	sender    mail139Sender
	logger    *logger.Logger
	dedup     MailSendDedupStore
	peers     []MailPeer
	userID    uint
	accountID uint
	phone     string
	auth      string
	message   string
	pending   bool
}

func NewMailMutualTask(client *corehttp.Client, log *logger.Logger) *MailMutualTask {
	upstream := api.NewCaiyunAPI(client)
	return &MailMutualTask{api: upstream, sender: upstream, logger: log}
}

func (t *MailMutualTask) SetAccount(userID, accountID uint, phone, auth string) *MailMutualTask {
	t.userID, t.accountID = userID, accountID
	t.phone, t.auth = strings.TrimSpace(phone), strings.TrimSpace(auth)
	return t
}

func (t *MailMutualTask) SetPeers(peers []MailPeer) *MailMutualTask {
	t.peers = peers
	return t
}

func (t *MailMutualTask) SetDedupStore(store MailSendDedupStore) *MailMutualTask {
	t.dedup = store
	return t
}

func (t *MailMutualTask) Message() string { return t.message }
func (t *MailMutualTask) Pending() bool   { return t.pending }

func mailMutualPeriod(now time.Time) (string, time.Duration) {
	zone := time.FixedZone("CST", 8*3600)
	local := now.In(zone)
	next := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, zone)
	return local.Format("200601"), next.Add(24 * time.Hour).Sub(now)
}

func mailMutualPairKey(userID, senderID, recipientID uint, period string) string {
	return fmt.Sprintf("mail139:mutual:v1:%d:%d:%d:%s", userID, senderID, recipientID, period)
}

func mailMutualClaimValue() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func mailMutualMaxSends() int {
	const fallback = 20
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CAIYUN_MAIL_MUTUAL_MAX_SENDS_PER_RUN"))); err == nil && value > 0 && value <= 500 {
		return value
	}
	return fallback
}

// Run sends one message from this account to each active peer belonging to the
// same platform user. A successful claim remains until next month; ambiguous
// transport failures also retain the claim to avoid duplicate real emails.
func (t *MailMutualTask) Run() error {
	t.pending = false
	t.logger.Start("------【139 邮箱账号互发】------")
	if t.dedup == nil {
		return fmt.Errorf("邮箱互发去重存储不可用")
	}
	if t.userID == 0 || t.accountID == 0 || t.phone == "" || t.auth == "" {
		return fmt.Errorf("邮箱互发缺少发件账号信息")
	}
	// The mailbox callback is only credited after the cloud task's front steps.
	// Register the current task before sending; a late registration loses credit.
	taskList, err := t.api.GetTaskListV3("newsign_139mail")
	if err != nil {
		return fmt.Errorf("读取邮箱任务前置步骤: %w", err)
	}
	needsBackup := false
	for _, task := range taskList {
		if strings.EqualFold(task.State, "FINISH") {
			continue
		}
		if task.ID == 1020 {
			needsBackup = true
		}
		if task.ID == 1004 || task.ID == 1020 {
			if err := t.api.DoTaskWithMarket("newsign_139mail", "task", strconv.Itoa(task.ID)); err != nil {
				return fmt.Errorf("邮箱任务前置登记: %w", err)
			}
		}
	}

	session, err := t.api.Login139Mail(t.phone, t.auth)
	if err != nil {
		return err
	}
	period, ttl := mailMutualPeriod(time.Now())
	limit := mailMutualMaxSends()
	start := (time.Now().In(time.FixedZone("CST", 8*3600)).Day() - 1) * limit
	sent, already, failed, failures, err := t.sendPairs(session, period, ttl, limit, start)
	if err != nil {
		return err
	}

	parts := []string{fmt.Sprintf("本月新发 %d 封，已处理 %d 对，失败 %d 对", sent, already, failed)}
	if len(t.peers) == 0 {
		parts = append(parts, "没有其他活跃账号，未发互助邮件")
	}
	if sent > 0 || already > 0 {
		if err := t.api.Report139MailTask(session); err != nil {
			parts = append(parts, "邮件已发出，云盘活动上报未成功: "+err.Error())
			t.pending = true
		}
	}
	if needsBackup {
		if err := t.api.Backup139Mail(session); err != nil {
			parts = append(parts, "邮箱转存未成功: "+err.Error())
			t.pending = true
		} else {
			parts = append(parts, "邮箱转存接口已接受")
		}
	}
	if sent > 0 || already > 0 || needsBackup {
		if latest, err := t.api.GetTaskListV3("newsign_139mail"); err == nil {
			for _, task := range latest {
				if (task.ID == 1004 || task.ID == 1020) && !strings.EqualFold(task.State, "FINISH") {
					parts = append(parts, fmt.Sprintf("邮箱任务%d待服务端确认", task.ID))
					t.pending = true
				}
			}
		} else {
			parts = append(parts, "无法复查邮箱任务状态: "+err.Error())
			t.pending = true
		}
	}
	t.message = strings.Join(parts, "；")
	if len(failures) > 0 {
		return fmt.Errorf("%s；%s", t.message, strings.Join(failures, "；"))
	}
	t.logger.Success(t.message)
	return nil
}

func (t *MailMutualTask) sendPairs(session *api.Mail139Session, period string, ttl time.Duration, limit, startIndex int) (sent, already, failed int, failures []string, err error) {
	sent, already, failed, attempted := 0, 0, 0, 0
	if t.sender == nil || t.dedup == nil {
		return 0, 0, 0, nil, fmt.Errorf("邮箱互发发送器或去重存储不可用")
	}
	for index := 0; index < len(t.peers); index++ {
		if attempted >= limit {
			break
		}
		peer := t.peers[(startIndex+index)%len(t.peers)]
		if peer.AccountID == 0 || peer.AccountID == t.accountID || peer.Phone == "" || peer.Phone == t.phone {
			continue
		}
		key := mailMutualPairKey(t.userID, t.accountID, peer.AccountID, period)
		claim, err := mailMutualClaimValue()
		if err != nil {
			return sent, already, failed, failures, err
		}
		claimed, err := t.dedup.SetNX(key, claim, ttl)
		if err != nil {
			return sent, already, failed, failures, fmt.Errorf("邮箱互发去重失败: %w", err)
		}
		if !claimed {
			already++
			continue
		}
		attempted++
		recipient := peer.Phone + "@139.com"
		err = t.sender.Send139Mail(session, t.phone, recipient,
			"移动云盘账号互助 "+period,
			"你好，这是一封由同一用户托管账号发出的互助任务邮件。")
		if err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%d: %v", peer.AccountID, err))
			if errors.Is(err, api.ErrMail139Rejected) {
				_, _ = t.dedup.DelIfValue(key, claim)
			}
			continue
		}
		sent++
	}
	return sent, already, failed, failures, nil
}
