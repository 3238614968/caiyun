package services

import (
	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"encoding/json"
	"fmt"
	"time"
)

// ProductService 商品管理服务。
type ProductService struct {
	productRepo *repository.ProductRepository
	accountRepo *repository.AccountRepository
}

// NewProductService 创建商品管理服务。
func NewProductService(productRepo *repository.ProductRepository, accountRepo *repository.AccountRepository) *ProductService {
	return &ProductService{
		productRepo: productRepo,
		accountRepo: accountRepo,
	}
}

// GetProducts 获取商品列表。
func (s *ProductService) GetProducts(keyword string, category string, limit int) ([]*models.Product, error) {
	if keyword != "" {
		return s.productRepo.Search(keyword, limit)
	}

	if category != "" {
		return s.productRepo.FindByCategory(category)
	}

	return s.productRepo.FindActive()
}

// GetCategories 获取商品分类。
func (s *ProductService) GetCategories() ([]string, error) {
	return s.productRepo.GetCategories()
}

// UpdateProducts 从云盘接口拉取商品并写入本地。
func (s *ProductService) UpdateProducts(accountID uint, userID uint, isAdmin bool) (int64, error) {
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return 0, fmt.Errorf("获取账号失败: %w", err)
	}
	if !isAdmin && account.UserID != userID {
		return 0, fmt.Errorf("账号不存在")
	}

	return syncProductsFromCloud(s.productRepo, s.accountRepo, accountID)
}

func syncProductsFromCloud(productRepo *repository.ProductRepository, accountRepo *repository.AccountRepository, accountID uint) (int64, error) {
	if productRepo == nil || accountRepo == nil {
		return 0, fmt.Errorf("商品或账号仓储未初始化")
	}

	account, err := accountRepo.GetByID(accountID)
	if err != nil {
		return 0, fmt.Errorf("获取账号失败: %w", err)
	}

	authStr := sanitizeAuthValue(account.Auth)
	if authStr == "" {
		return 0, fmt.Errorf("账号 Auth 为空")
	}

	authClient := corehttp.NewClient()
	authClient.SetAuth(authStr)
	jwtToken, ssoToken, err := auth.NewAuth(authClient).GetJWTTokenWithSSOToken(account.Phone)
	if err != nil {
		return 0, fmt.Errorf("获取账号 JWT 失败: %w", err)
	}
	if jwtToken == "" {
		return 0, fmt.Errorf("账号 JWT 为空")
	}

	client := corehttp.NewClient()
	client.SetMarketAccount(account.Phone)
	client.SetAuth(authStr)
	if ssoToken != "" {
		client.SetSSOToken(ssoToken)
	}
	client.SetJWTToken(jwtToken)

	resp, err := api.NewCaiyunAPI(client).GetProductList()
	if err != nil {
		return 0, fmt.Errorf("获取商品列表失败: %w", err)
	}

	code := 0
	switch v := resp.Code.(type) {
	case int:
		code = v
	case float64:
		code = int(v)
	case string:
		if v != "0" {
			code = 1
		}
	default:
		code = 1
	}
	if code != 0 {
		msg := resp.Message
		if msg == "" {
			msg = resp.Msg
		}
		return 0, fmt.Errorf("商品列表返回失败: %s", msg)
	}

	type prizeInfo struct {
		PrizeName           string `json:"prizeName"`
		POrder              int    `json:"pOrder"`
		DailyRemainderCount int    `json:"dailyRemainderCount"`
		DailyLimitCount     int    `json:"dailyLimitCount"`
		DailyCount          int    `json:"dailyCount"`
		Memo                string `json:"memo"`
		PrizeID             int    `json:"prizeId"`
		ImageURL            string `json:"imageUrl"`
	}

	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return 0, fmt.Errorf("序列化商品数据失败: %w", err)
	}

	var grouped map[string][]prizeInfo
	if err := json.Unmarshal(resultBytes, &grouped); err != nil {
		return 0, fmt.Errorf("解析商品数据失败: %w", err)
	}

	categoryMap := map[string]string{
		"0":  "其他权益奖品",
		"1":  "视频类会员",
		"2":  "音乐类会员",
		"5":  "外卖美食权益",
		"7":  "快递寄件券",
		"8":  "云盘转存券",
		"9":  "实用工具类",
		"10": "奶茶饮品权益",
		"11": "奶茶饮品权益",
		"13": "咖啡饮品权益",
		"14": "游戏礼包权益",
		"15": "全国通用流量权益",
	}

	now := time.Now()
	products := make([]*models.Product, 0, 128)
	for categoryID, prizes := range grouped {
		category := categoryMap[categoryID]
		if category == "" {
			category = "未知分类" + categoryID
		}

		for _, item := range prizes {
			prizeID := ""
			if item.PrizeID > 0 {
				prizeID = fmt.Sprintf("%d", item.PrizeID)
			}
			if prizeID == "" {
				prizeID = item.Memo
			}
			if prizeID == "" {
				continue
			}

			stockStatus := "sold_out"
			if item.DailyRemainderCount > 0 {
				stockStatus = "available"
			}

			products = append(products, &models.Product{
				PrizeID:             prizeID,
				PrizedName:          item.PrizeName,
				POrder:              item.POrder,
				Category:            category,
				DailyRemainderCount: item.DailyRemainderCount,
				DailyLimitCount:     item.DailyLimitCount,
				DailyCount:          item.DailyCount,
				ImageURL:            item.ImageURL,
				StockStatus:         stockStatus,
				LastStockCheck:      &now,
				Memo:                item.Memo,
				IsActive:            true,
				IsDeleted:           false,
			})
		}
	}

	if len(products) == 0 {
		return 0, fmt.Errorf("未获取到任何商品数据")
	}

	updated, inserted, disabled, syncedTasks, stoppedTasks, err := productRepo.ReplaceProducts(products)
	if err != nil {
		return 0, fmt.Errorf("保存商品失败: %w", err)
	}
	fmt.Printf("商品列表已刷新：最新商品 %d 个，更新 %d 个，新增 %d 个，下架旧商品 %d 个，同步抢兑任务 %d 个，停止失效任务 %d 个\n",
		len(products), updated, inserted, disabled, syncedTasks, stoppedTasks)

	return int64(len(products)), nil
}
