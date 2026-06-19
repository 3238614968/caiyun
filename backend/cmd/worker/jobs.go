package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/notification"
	"caiyun/internal/repository"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"
)

// registerMonthlyExchangeJob 注册自动兑换月卡定时任务
func registerMonthlyExchangeJob(scheduler *scheduler.Scheduler, exchangeService *services.ExchangeService, configRepo *repository.SystemConfigRepository) {
	// 获取兑换时间配置，默认10:00
	exchangeTime := "10:00"
	if config, err := configRepo.GetByKey("exchange_monthly_time"); err == nil && config.KeyValue != "" {
		exchangeTime = config.KeyValue
	}

	// 解析时间 (HH:MM)
	parts := strings.Split(exchangeTime, ":")
	if len(parts) != 2 {
		log.Printf("【月卡兑换】时间格式错误: %s，使用默认时间 10:00", exchangeTime)
		parts = []string{"10", "00"}
	}
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])

	// 构建 cron 表达式
	cronExpr := fmt.Sprintf("%d %d * * *", minute, hour)

	_, err := scheduler.AddJobWithName(
		"monthly_exchange",
		cronExpr,
		func() error {
			// 检查是否启用自动兑换
			if config, err := configRepo.GetByKey("exchange_monthly_enabled"); err == nil {
				enabled := config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
				if !enabled {
					log.Println("【月卡兑换】自动兑换已禁用，跳过执行")
					return nil
				}
			}

			log.Println("【月卡兑换】开始执行自动兑换月卡任务...")
			exchangeService.ExecuteMonthlyExchange()
			return nil
		},
		"自动兑换月卡任务",
	)
	if err != nil {
		log.Printf("【月卡兑换】添加定时任务失败: %v", err)
	} else {
		log.Printf("【月卡兑换】定时任务已注册，执行时间: %02d:%02d", hour, minute)
	}
}

// registerAutoUpdateProductsJob 注册商品自动更新定时任务
func registerAutoUpdateProductsJob(scheduler *scheduler.Scheduler, exchangeService *services.ExchangeService, configRepo *repository.SystemConfigRepository, accountRepo *repository.AccountRepository) {
	// 每天凌晨3点更新商品
	cronExpr := constants.AutoUpdateProductsCron // 使用常量：每天凌晨3点

	_, err := scheduler.AddJobWithName(
		"auto_update_products",
		cronExpr,
		func() error {
			// 检查是否启用自动更新
			if config, err := configRepo.GetByKey("exchange_auto_update_products"); err == nil {
				enabled := config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
				if !enabled {
					log.Println("【商品更新】自动更新已禁用，跳过执行")
					return nil
				}
			}

			log.Println("【商品更新】开始执行自动更新商品任务...")

			// 获取一个有效的账号作为数据源
			accounts, err := accountRepo.GetAllActive()
			if err != nil || len(accounts) == 0 {
				log.Printf("【商品更新】没有可用的账号，跳过更新: %v", err)
				return nil
			}

			// 使用第一个账号更新商品
			if err := exchangeService.UpdateProducts(accounts[0].ID); err != nil {
				log.Printf("【商品更新】更新商品失败: %v", err)
				return err
			}

			log.Println("【商品更新】商品更新成功")
			return nil
		},
		"自动更新商品列表任务",
	)
	if err != nil {
		log.Printf("【商品更新】添加定时任务失败: %v", err)
	} else {
		log.Println("【商品更新】定时任务已注册，执行时间: 每天 03:00")
	}
}

// registerAccountHealthCheckJob 注册账号健康检查定时任务
func registerAccountHealthCheckJob(scheduler *scheduler.Scheduler, tokenManager *services.TokenManager, accountRepo *repository.AccountRepository, notifier notification.Notifier) {
	// 每6小时检查一次账号健康状态
	cronExpr := constants.AccountHealthCheckCron // 使用常量：每6小时
	var running atomic.Bool

	_, err := scheduler.AddJobWithName(
		"account_health_check",
		cronExpr,
		func() error {
			if !running.CompareAndSwap(false, true) {
				log.Println("【账号检测】上一轮健康检查仍在执行，跳过本轮")
				return nil
			}
			defer running.Store(false)

			log.Println("【账号检测】开始执行账号健康检查...")

			healthyCount := 0
			unhealthyCount := 0
			const batchSize = 200

			for offset := 0; ; offset += batchSize {
				accounts, err := accountRepo.FindActiveAccountsPaged(offset, batchSize)
				if err != nil {
					log.Printf("【账号检测】获取账号列表失败: %v", err)
					return err
				}
				if len(accounts) == 0 {
					break
				}

				var wg sync.WaitGroup
				var countMu sync.Mutex
				sem := make(chan struct{}, 5)
				for _, account := range accounts {
					account := account
					wg.Add(1)
					go func() {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()

						healthy, unhealthy := checkAccountHealth(tokenManager, account, notifier)
						countMu.Lock()
						healthyCount += healthy
						unhealthyCount += unhealthy
						countMu.Unlock()
					}()
				}
				wg.Wait()

				if len(accounts) < batchSize {
					break
				}
			}

			log.Printf("【账号检测】检查完成，健康账号: %d，异常账号: %d", healthyCount, unhealthyCount)
			return nil
		},
		"账号健康检查任务",
	)
	if err != nil {
		log.Printf("【账号检测】添加定时任务失败: %v", err)
	} else {
		log.Println("【账号检测】定时任务已注册，执行时间: 每6小时")
	}
}

func checkAccountHealth(tokenManager *services.TokenManager, account *models.Account, notifier notification.Notifier) (healthyCount int, unhealthyCount int) {
	if account == nil {
		return 0, 0
	}

	// 使用 TokenManager 检查账号 Token 状态
	tokenInfo, err := tokenManager.GetToken(account.ID)
	phoneForLog := maskPhoneForLog(account.Phone)
	if err != nil {
		log.Printf("【账号检测】账号 %s (ID: %d) Token 获取失败: %v", phoneForLog, account.ID, err)
		if notifier != nil {
			notifier.SendTaskFailure(account.ID, "health_check", fmt.Sprintf("Token 无效: %v", err))
		}
		return 0, 1
	}

	// 检查 Token 健康状态
	if tokenInfo.HealthStatus == "healthy" {
		log.Printf("【账号检测】账号 %s (ID: %d) 健康状态良好", phoneForLog, account.ID)
		return 1, 0
	}

	log.Printf("【账号检测】账号 %s (ID: %d) 健康状态异常: %s", phoneForLog, account.ID, tokenInfo.ErrorMsg)
	if notifier != nil {
		notifier.SendTaskFailure(account.ID, "health_check", tokenInfo.ErrorMsg)
	}

	// 尝试强制刷新 Token
	if _, err := tokenManager.ForceRefresh(account.ID); err != nil {
		log.Printf("【账号检测】账号 %s (ID: %d) Token 刷新失败: %v", phoneForLog, account.ID, err)
	} else {
		log.Printf("【账号检测】账号 %s (ID: %d) Token 刷新成功", phoneForLog, account.ID)
	}
	return 0, 1
}

func maskPhoneForLog(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) < 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}
