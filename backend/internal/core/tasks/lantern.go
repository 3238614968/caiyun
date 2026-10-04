package tasks

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

const lanternDailyDrawLimit = 4

// LanternTask 按已知谜底答题；未知/图片题跳过，答错不能进入抽奖。
type LanternTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	delay       time.Duration
	lastMessage string
	pending     bool
}

func NewLanternTask(client *http.Client, log *logger.Logger) *LanternTask {
	return &LanternTask{api: api.NewCaiyunAPI(client), logger: log, delay: time.Second}
}

func (t *LanternTask) Message() string { return t.lastMessage }
func (t *LanternTask) Pending() bool   { return t.pending }

func (t *LanternTask) Run() error {
	t.lastMessage, t.pending = "", false
	puzzles, err := t.api.LanternPuzzles()
	if err != nil {
		return err
	}
	draws, wins, skipped := 0, 0, 0
	limitReached := false
	var prizes []string
	seen := map[string]bool{}
puzzleLoop:
	for _, puzzle := range puzzles {
		if draws >= lanternDailyDrawLimit {
			limitReached = true
			break
		}
		id := puzzle.IDKey()
		answers := lanternAnswers(puzzle.PuzzleTitleContext)
		if id == "" || len(answers) == 0 {
			skipped++
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		correct := false
		for _, answer := range answers {
			resp, err := t.api.LanternSubmit(puzzle.ID, answer)
			if err != nil {
				return err
			}
			if responseCodeIs(resp, 201) {
				limitReached = true
				break puzzleLoop
			}
			if responseCodeIs(resp, 0, 203) {
				correct = true
				break
			}
			if !responseCodeIs(resp, 202) {
				return fmt.Errorf("灯谜答题失败: code=%v %s", resp.Code, resp.MessageText())
			}
			t.pause()
		}
		if !correct {
			skipped++
			continue
		}
		t.pause()
		resp, err := t.api.LanternDraw(puzzle.ID)
		if err != nil {
			return err
		}
		if !responseCodeIs(resp, 0, 10010020) {
			return fmt.Errorf("灯谜抽奖失败: code=%v %s", resp.Code, resp.MessageText())
		}
		draws++
		if responseCodeIs(resp, 0) {
			result, _ := resp.Result.(map[string]interface{})
			if name, _ := result["prizeName"].(string); name != "" {
				wins++
				prizes = append(prizes, name)
			}
		}
		t.pause()
	}
	t.pending = draws < lanternDailyDrawLimit && skipped > 0 && !limitReached
	t.lastMessage = fmt.Sprintf("灯谜抽奖%d/%d次、中奖%d次、跳过%d题（无已知谜底/未答对）", draws, lanternDailyDrawLimit, wins, skipped)
	if limitReached {
		t.lastMessage += "；已达今日机会限制"
	}
	if len(prizes) > 0 {
		t.lastMessage += "；" + strings.Join(prizes, "、") + "（按奖品指引领取）"
	}
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return nil
}

func (t *LanternTask) pause() {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
}

func normalizeLanternClue(clue string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || (unicode.IsPunct(r) && r != '_') {
			return -1
		}
		return r
	}, clue)
}

func lanternAnswers(clue string) []string {
	clue = normalizeLanternClue(clue)
	if utf8.RuneCountInString(clue) < 2 {
		return nil
	}
	if answers, ok := lanternAnswerBook[clue]; ok {
		return answers
	}
	// 只匹配完整已知谜面，选最长匹配，避免把短文本误配到无关题目。
	best := ""
	for key := range lanternAnswerBook {
		if strings.Contains(clue, key) && (len(key) > len(best) || (len(key) == len(best) && key < best)) {
			best = key
		}
	}
	return lanternAnswerBook[best]
}

var lanternAnswerBook = map[string][]string{
	"零存整取": {"积少成多", "集腋成裘", "日积月累"},
	"锣鼓喧天人欢笑彩旗飘扬映日辉": {"普天同庆", "欢天喜地", "举国欢庆", "张灯结彩"},
	"一人扛两土": {"佳", "王", "全", "坐"},
	"万古长__": {"青"}, "万古长青": {"青"},
	"春花__月": {"秋"}, "春花秋月": {"秋"},
	"精忠报__": {"国"}, "精忠报国": {"国"},
	"山水甲天下":         {"桂林", "阳朔"},
	"耳朵长尾巴短爱吃萝卜爱吃菜": {"兔子", "兔"},
	"十五日":           {"胖", "半"}, "十五的月亮": {"圆"}, "举头望明月": {"思乡"},
	"夏天穿棉袄": {"武汉"}, "五岳之中它最陡云梯直上惊心走": {"华山"},
	"不冷不热的地方": {"温州"}, "桂林山水甲____": {"天下"},
	"桂林山水谁称甲一幅画卷漂江下": {"漓江", "桂林漓江"},
	"八月开花似小米一到中秋香气散": {"桂花"}, "一寸光阴一寸金": {"贵阳"},
	"举国同庆华诞日": {"国庆节", "国庆"}, "世界最高的山峰": {"珠穆朗玛峰", "珠峰"},
}
