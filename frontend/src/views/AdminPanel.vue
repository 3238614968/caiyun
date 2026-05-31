<template>
  <div class="admin-panel-container">
    <el-card shadow="hover" class="admin-shell">
      <template #header>
        <div class="card-header">
          <span>管理员面板</span>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="handleTabChange" class="admin-tabs">
        <!-- 账号概况 -->
        <el-tab-pane label="账号概况" name="summaries">
          <div class="tab-content">
            <div class="responsive-data-shell" v-loading="summaryLoading">
<el-table v-if="!isMobile" :data="summaryList" stripe style="width: 100%">
              <el-table-column prop="phone" label="手机号" width="130" />
              <el-table-column prop="owner_username" label="所属用户" width="100" />
              <el-table-column prop="cloud_count" label="当前云朵" width="100">
                <template #default="{ row }">
                  <span style="font-weight: 600; color: #3b82f6">{{ row.cloud_count }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="today_gained" label="今日获得" width="100">
                <template #default="{ row }">
                  <span v-if="row.today_gained > 0" style="color: #10b981">+{{ row.today_gained }}</span>
                  <span v-else>0</span>
                </template>
              </el-table-column>
              <el-table-column prop="yesterday_gained" label="昨日获得" width="100">
                <template #default="{ row }">
                  <span v-if="row.yesterday_gained > 0" style="color: #10b981">+{{ row.yesterday_gained }}</span>
                  <span v-else>0</span>
                </template>
              </el-table-column>
              <el-table-column label="今日任务" width="140">
                <template #default="{ row }">
                  <el-tag type="success" size="small">成功 {{ row.success_count }}</el-tag>
                  <el-tag v-if="row.failed_count > 0" type="danger" size="small" style="margin-left: 4px">
                    失败 {{ row.failed_count }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="80">
                <template #default="{ row }">
                  <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
                    {{ row.is_active ? '激活' : '停用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="last_executed_at" label="最后执行" width="160">
                <template #default="{ row }">
                  {{ row.last_executed_at || '-' }}
                </template>
              </el-table-column>
              <el-table-column prop="remark" label="备注" />
              <el-table-column prop="created_at" label="添加时间" width="160" />
            </el-table>

              <div v-else class="mobile-admin-list">
                <el-empty v-if="summaryList.length === 0" description="暂无账号概况" />
                <template v-else>
                  <el-card
                    v-for="row in summaryList"
                    :key="`${row.phone}-${row.owner_username}-${row.created_at}`"
                    class="mobile-admin-card mobile-summary-card"
                    shadow="never"
                  >
                    <div class="mobile-admin-card-head">
                      <div>
                        <div class="mobile-admin-card-title">{{ row.phone }}</div>
                        <div class="mobile-admin-card-meta">{{ row.owner_username || '-' }}</div>
                      </div>
                      <el-tag :type="row.is_active ? 'success' : 'info'" effect="light">{{ row.is_active ? '激活' : '停用' }}</el-tag>
                    </div>
                    <div class="mobile-admin-card-grid">
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">当前云朵</span>
                        <span class="mobile-admin-card-value strong">{{ row.cloud_count }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">今日获得</span>
                        <span class="mobile-admin-card-value success">{{ row.today_gained > 0 ? `+${row.today_gained}` : '0' }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">昨日获得</span>
                        <span class="mobile-admin-card-value">{{ row.yesterday_gained > 0 ? `+${row.yesterday_gained}` : '0' }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">最后执行</span>
                        <span class="mobile-admin-card-value">{{ row.last_executed_at || '-' }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">备注</span>
                        <span class="mobile-admin-card-value">{{ row.remark || '-' }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">添加时间</span>
                        <span class="mobile-admin-card-value">{{ row.created_at }}</span>
                      </div>
                    </div>
                    <div class="mobile-admin-inline-tags">
                      <el-tag type="success" size="small">成功 {{ row.success_count }}</el-tag>
                      <el-tag v-if="row.failed_count > 0" type="danger" size="small">失败 {{ row.failed_count }}</el-tag>
                    </div>
                  </el-card>
                </template>
              </div>
            </div>
            <el-pagination
              v-model:current-page="summaryPagination.page"
              v-model:page-size="summaryPagination.pageSize"
              :page-sizes="[20, 50, 100]"
              :total="summaryPagination.total"
              layout="total, sizes, prev, pager, next"
              @size-change="(s: number) => { summaryPagination.pageSize = s; loadSummaries() }"
              @current-change="(p: number) => { summaryPagination.page = p; loadSummaries() }"
              style="margin-top: 20px"
            />
          </div>
        </el-tab-pane>

        <!-- 任务管理 -->
        <el-tab-pane label="任务管理" name="tasks">
          <div class="tab-content">
            <p style="color: #666; margin-bottom: 16px">
              下架的任务将不会被手动执行和定时任务执行。
            </p>
            <div class="responsive-data-shell" v-loading="taskConfigLoading">
<el-table v-if="!isMobile" :data="taskConfigs" stripe style="width: 100%">
              <el-table-column prop="sort_order" label="序号" width="70" />
              <el-table-column prop="task_name" label="任务名称" width="120" />
              <el-table-column prop="task_type" label="任务类型" width="140">
                <template #default="{ row }">
                  <el-tag size="small">{{ getTaskTypeName(row.task_type, row.task_name) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="160">
                <template #default="{ row }">
                  <div class="task-status-cell">
                    <el-switch
                      v-model="row.is_enabled"
                      @change="handleTaskConfigChange(row)"
                    />
                    <span :class="row.is_enabled ? 'status-on' : 'status-off'">
                      {{ row.is_enabled ? '已上架' : '已下架' }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="updated_at" label="更新时间" min-width="180">
                <template #default="{ row }">
                  {{ formatDate(row.updated_at) }}
                </template>
              </el-table-column>
            </el-table>

              <div v-else class="mobile-admin-list">
                <el-empty v-if="taskConfigs.length === 0" description="暂无任务配置" />
                <template v-else>
                  <el-card
                    v-for="row in taskConfigs"
                    :key="row.task_type"
                    class="mobile-admin-card"
                    shadow="never"
                  >
                    <div class="mobile-admin-card-head">
                      <div>
                        <div class="mobile-admin-card-title">{{ row.task_name }}</div>
                        <div class="mobile-admin-card-meta">#{{ row.sort_order }}</div>
                      </div>
                      <el-tag size="small">{{ getTaskTypeName(row.task_type, row.task_name) }}</el-tag>
                    </div>
                    <div class="mobile-admin-card-grid">
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">任务类型</span>
                        <span class="mobile-admin-card-value">{{ getTaskTypeName(row.task_type, row.task_name) }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">更新时间</span>
                        <span class="mobile-admin-card-value">{{ formatDate(row.updated_at) }}</span>
                      </div>
                    </div>
                    <div class="mobile-admin-card-footer">
                      <div class="task-status-cell">
                        <el-switch v-model="row.is_enabled" @change="handleTaskConfigChange(row)" />
                        <span :class="row.is_enabled ? 'status-on' : 'status-off'">{{ row.is_enabled ? '已上架' : '已下架' }}</span>
                      </div>
                    </div>
                  </el-card>
                </template>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <!-- 抢兑配置 -->
        <el-tab-pane label="抢兑配置" name="exchange">
          <div class="tab-content">
            <el-row :gutter="20">
              <!-- 抢兑基础配置 -->
              <el-col :span="12">
                <el-card shadow="hover" class="config-card">
                  <template #header>
                    <div class="config-header">
                      <span>基础配置</span>
                    </div>
                  </template>
                  <el-form :model="exchangeConfig" label-width="150px">
                    <el-form-item label="抢兑功能开关">
                      <el-switch v-model="exchangeConfig.enabled" />
                    </el-form-item>
                    <el-form-item label="自动更新商品库">
                      <el-switch v-model="exchangeConfig.auto_update_products" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">每天早上 8 点自动更新</span>
                    </el-form-item>
                    <el-form-item label="抢兑并发数" required>
                      <el-input-number v-model="exchangeConfig.concurrency" :min="1" :max="50" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">同时执行的抢兑任务数</span>
                    </el-form-item>
                    <el-form-item label="立即兑换功能">
                      <el-switch v-model="exchangeConfig.immediate_exchange_enabled" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">启用后用户可直接兑换，无需创建任务</span>
                    </el-form-item>
                    <el-form-item>
                      <el-button type="primary" @click="saveExchangeConfig">保存配置</el-button>
                    </el-form-item>
                  </el-form>
                </el-card>
              </el-col>

              <!-- 兑换月卡配置 -->
              <el-col :span="12">
                <el-card shadow="hover" class="config-card">
                  <template #header>
                    <div class="config-header">
                      <span>兑换月卡配置</span>
                      <el-tag v-if="exchangeConfig.exchange_monthly_enabled" type="success">已启用</el-tag>
                      <el-tag v-else type="info">已禁用</el-tag>
                    </div>
                  </template>
                  <el-form :model="exchangeConfig" label-width="150px">
                    <el-form-item label="兑换月卡开关">
                      <el-switch v-model="exchangeConfig.exchange_monthly_enabled" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">启用后自动兑换月卡</span>
                    </el-form-item>
                    <el-form-item label="自动兑换时间">
                      <el-time-picker
                        v-model="exchangeConfig.exchange_time"
                        format="HH:mm"
                        value-format="HH:mm"
                        placeholder="选择时间"
                        style="width: 100%;"
                      />
                    </el-form-item>
                    <el-form-item label="月卡商品ID">
                      <el-input
                        v-model="exchangeConfig.monthly_prize_id"
                        placeholder="请输入月卡商品ID"
                        style="width: 100%;"
                      />
                      <span style="font-size: 12px; color: #999;">默认1001，可从商品中心查看</span>
                    </el-form-item>
                    <el-form-item>
                      <el-button type="primary" @click="saveExchangeConfig">保存配置</el-button>
                      <el-button type="success" @click="executeMonthlyExchange" :loading="monthlyExchangeLoading">
                        立即执行兑换
                      </el-button>
                    </el-form-item>
                  </el-form>
                </el-card>
              </el-col>
            </el-row>

            <!-- 商品中心管理 -->
            <el-card shadow="hover" class="config-card" style="margin-top: 20px;">
              <template #header>
                <div class="config-header">
                  <span>商品中心管理</span>
                </div>
              </template>
              <div class="product-management">
                <p style="color: #666; margin-bottom: 16px;">
                  手动更新商品中心数据，需要选择一个有效的云盘账号作为数据获取源。
                </p>
                <el-form :inline="true">
                  <el-form-item label="选择账号">
                    <el-select
                      v-model="selectedAccountId"
                      placeholder="请选择云盘账号"
                      style="width: 250px;"
                      :disabled="availableProductSourceAccounts.length === 0"
                    >
                      <el-option
                        v-for="acc in availableProductSourceAccounts"
                        :key="acc.id"
                        :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
                        :value="acc.id"
                      />
                    </el-select>
                  </el-form-item>
                  <el-form-item>
                    <el-button type="primary" @click="handleUpdateProducts" :loading="updateProductsLoading">
                      <el-icon><Refresh /></el-icon>
                      更新商品数据
                    </el-button>
                  </el-form-item>
                </el-form>
              </div>
            </el-card>
          </div>
        </el-tab-pane>

        <!-- 用户管理 -->
        <el-tab-pane label="用户管理" name="users">
          <div class="tab-content">
            <div class="responsive-data-shell" v-loading="userLoading">
<el-table v-if="!isMobile" :data="userList" stripe style="width: 100%">
              <el-table-column prop="username" label="用户名" width="150" />
              <el-table-column prop="email" label="邮箱" />
              <el-table-column prop="role" label="角色" width="120">
                <template #default="{ row }">
                  <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">
                    {{ row.role === 'admin' ? '管理员' : '普通用户' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" label="创建时间" width="180">
                <template #default="{ row }">
                  {{ formatDate(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="220" fixed="right">
                <template #default="{ row }">
                  <el-button type="primary" link @click="handleEditUserRole(row)">修改角色</el-button>
                  <el-button type="warning" link @click="handleResetUserPassword(row)">重置密码</el-button>
                  <el-button type="danger" link @click="handleDeleteUser(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>

              <div v-else class="mobile-admin-list">
                <el-empty v-if="userList.length === 0" description="暂无用户数据" />
                <template v-else>
                  <el-card
                    v-for="row in userList"
                    :key="row.id"
                    class="mobile-admin-card"
                    shadow="never"
                  >
                    <div class="mobile-admin-card-head">
                      <div>
                        <div class="mobile-admin-card-title">{{ row.username }}</div>
                        <div class="mobile-admin-card-meta">{{ row.email || '-' }}</div>
                      </div>
                      <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">{{ row.role === 'admin' ? '管理员' : '普通用户' }}</el-tag>
                    </div>
                    <div class="mobile-admin-card-grid">
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">邮箱</span>
                        <span class="mobile-admin-card-value">{{ row.email || '-' }}</span>
                      </div>
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">创建时间</span>
                        <span class="mobile-admin-card-value">{{ formatDate(row.created_at) }}</span>
                      </div>
                    </div>
                    <div class="mobile-admin-card-actions">
                      <el-button type="primary" plain @click="handleEditUserRole(row)">修改角色</el-button>
                      <el-button type="warning" plain @click="handleResetUserPassword(row)">重置密码</el-button>
                      <el-button type="danger" plain @click="handleDeleteUser(row)">删除</el-button>
                    </div>
                  </el-card>
                </template>
              </div>
            </div>
            <el-pagination
              v-model:current-page="userPagination.page"
              v-model:page-size="userPagination.pageSize"
              :page-sizes="[10, 20, 50]"
              :total="userPagination.total"
              layout="total, sizes, prev, pager, next"
              @size-change="(s: number) => { userPagination.pageSize = s; loadUserList() }"
              @current-change="(p: number) => { userPagination.page = p; loadUserList() }"
              style="margin-top: 20px"
            />
          </div>
        </el-tab-pane>

        <!-- 统计概览 -->
        <el-tab-pane label="统计概览" name="stats">
          <div class="tab-content">
            <el-row :gutter="20">
              <el-col :span="6" v-for="stat in statsOverview" :key="stat.key">
                <el-card shadow="hover" class="stat-card">
                  <div class="stat-content">
                    <div class="stat-icon" :style="{ background: stat.color }">
                      <el-icon><component :is="stat.icon" /></el-icon>
                    </div>
                    <div class="stat-info">
                      <div class="stat-value">{{ stat.value }}</div>
                      <div class="stat-label">{{ stat.label }}</div>
                    </div>
                  </div>
                </el-card>
              </el-col>
            </el-row>
          </div>
        </el-tab-pane>

        <!-- 公告管理 -->
        <el-tab-pane label="公告管理" name="announcements">
          <div class="tab-content announcement-content">
            <div class="announcement-header">
              <el-button type="primary" @click="showAddAnnouncementDialog">
                <el-icon><Plus /></el-icon>
                发布公告
              </el-button>
            </div>
            <div class="responsive-data-shell" v-loading="announcementLoading">
<el-table v-if="!isMobile" :data="announcements" stripe style="width: 100%">
              <el-table-column type="index" width="50" />
              <el-table-column prop="title" label="标题" min-width="200">
                <template #default="{ row }">
                  <div class="title-cell">
                    <el-tag v-if="row.is_top" type="danger" size="small" effect="dark">置顶</el-tag>
                    <el-tag v-if="row.is_popup" type="warning" size="small" class="ml-2">弹窗</el-tag>
                    <span class="title-text">{{ row.title }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="is_published" label="状态" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.is_published ? 'success' : 'info'">
                    {{ row.is_published ? '已发布' : '已下架' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" label="创建时间" width="180">
                <template #default="{ row }">
                  {{ formatDate(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="200" fixed="right">
                <template #default="{ row }">
                  <el-button size="small" @click="viewAnnouncement(row)">查看</el-button>
                  <el-button size="small" type="primary" @click="editAnnouncement(row)">编辑</el-button>
                  <el-button size="small" type="danger" @click="deleteAnnouncement(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>

              <div v-else class="mobile-admin-list">
                <el-empty v-if="announcements.length === 0" description="暂无公告" />
                <template v-else>
                  <el-card
                    v-for="row in announcements"
                    :key="row.id"
                    class="mobile-admin-card"
                    shadow="never"
                  >
                    <div class="mobile-admin-card-head">
                      <div>
                        <div class="mobile-admin-card-title">{{ row.title }}</div>
                        <div class="mobile-admin-inline-tags">
                          <el-tag v-if="row.is_top" type="danger" size="small" effect="dark">置顶</el-tag>
                          <el-tag v-if="row.is_popup" type="warning" size="small">弹窗</el-tag>
                        </div>
                      </div>
                      <el-tag :type="row.is_published ? 'success' : 'info'">{{ row.is_published ? '已发布' : '已下架' }}</el-tag>
                    </div>
                    <div class="mobile-admin-card-grid">
                      <div class="mobile-admin-card-row">
                        <span class="mobile-admin-card-label">创建时间</span>
                        <span class="mobile-admin-card-value">{{ formatDate(row.created_at) }}</span>
                      </div>
                      <div class="mobile-admin-card-row full">
                        <span class="mobile-admin-card-label">内容预览</span>
                        <span class="mobile-admin-card-value multiline">{{ row.content || '-' }}</span>
                      </div>
                    </div>
                    <div class="mobile-admin-card-actions">
                      <el-button @click="viewAnnouncement(row)">查看</el-button>
                      <el-button type="primary" @click="editAnnouncement(row)">编辑</el-button>
                      <el-button type="danger" @click="deleteAnnouncement(row)">删除</el-button>
                    </div>
                  </el-card>
                </template>
              </div>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 修改角色对话框 -->
    <el-dialog v-model="roleDialogVisible" title="修改用户角色" width="400px">
      <el-form :model="roleForm" label-width="80px">
        <el-form-item label="角色">
          <el-radio-group v-model="roleForm.role">
            <el-radio label="user">普通用户</el-radio>
            <el-radio label="admin">管理员</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="roleDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleRoleSubmit">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码对话框 -->
    <el-dialog v-model="passwordDialogVisible" title="重置用户密码" width="420px">
      <el-form :model="passwordForm" label-width="90px">
        <el-form-item label="用户">
          <el-input v-model="passwordForm.username" disabled />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="passwordForm.password" type="password" show-password placeholder="至少12位，包含至少三类字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handlePasswordSubmit">确定重置</el-button>
      </template>
    </el-dialog>

    <!-- 公告管理对话框 -->
    <el-dialog v-model="announcementDialogVisible" :title="isEditingAnnouncement ? '编辑公告' : '发布公告'" width="700px">
      <el-form :model="announcementForm" label-position="top" :rules="announcementRules" ref="announcementFormRef">
        <el-form-item label="公告标题" prop="title">
          <el-input v-model="announcementForm.title" placeholder="请输入公告标题" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="公告内容" prop="content">
          <el-input
            v-model="announcementForm.content"
            type="textarea"
            :rows="6"
            placeholder="请输入公告内容"
            maxlength="2000"
            show-word-limit
          />
        </el-form-item>
        <el-form-item>
          <div class="form-options">
            <el-checkbox v-model="announcementForm.is_popup" label="弹窗显示" border />
            <el-checkbox v-model="announcementForm.is_top" label="置顶" border />
            <el-checkbox v-if="isEditingAnnouncement" v-model="announcementForm.is_published" label="发布状态" border />
          </div>
        </el-form-item>
        <el-form-item v-if="announcementForm.is_popup" class="tip-item">
          <el-alert
            title="弹窗公告说明"
            type="info"
            :closable="false"
            description="开启弹窗后，仅置顶且未读的弹窗公告会自动弹出一次；其他已发布公告会展示在首页公告列表中。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="announcementDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAnnouncementForm" :loading="announcementSubmitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 查看公告对话框 -->
    <el-dialog v-model="viewAnnouncementVisible" title="公告详情" width="600px" class="view-dialog">
      <div class="view-content">
        <h3 class="view-title">{{ currentAnnouncement?.title }}</h3>
        <div class="view-meta">
          <el-tag v-if="currentAnnouncement?.is_top" type="danger" size="small">置顶</el-tag>
          <el-tag v-if="currentAnnouncement?.is_popup" type="warning" size="small">弹窗</el-tag>
          <span class="view-time">{{ formatDate(currentAnnouncement?.created_at) }}</span>
        </div>
        <div class="view-body">{{ currentAnnouncement?.content }}</div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus } from '@element-plus/icons-vue'
import { type User } from '../api/auth'
import {
  type Account,
  type AccountSummary,
  type TaskConfig,
  getAllUsers,
  getAllAccounts,
  getAccountSummaries,
  getTaskConfigs,
  updateTaskConfig,
  updateUserRole,
  resetUserPassword,
  updateAccountStatus,
  deleteUser,
  deleteAdminAccount,
  getStatsOverview
} from '../api/account'
import {
  type Announcement,
  type CreateAnnouncementRequest,
  type UpdateAnnouncementRequest,
  getAllAnnouncements,
  createAnnouncement,
  updateAnnouncement,
  deleteAnnouncement as apiDeleteAnnouncement
} from '../api/announcement'
import {
  type ExchangeConfig,
  getExchangeConfig,
  updateExchangeConfig,
  updateProducts as apiUpdateProducts,
  executeMonthlyExchange as apiExecuteMonthlyExchange
} from '../api/exchange'
import { getTaskTypeName } from '../utils/task-types'

const activeTab = ref('summaries')
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const isMobile = computed(() => viewportWidth.value <= 768)

const syncViewport = () => {
  viewportWidth.value = window.innerWidth
}

// Account summaries
const summaryLoading = ref(false)
const summaryList = ref<AccountSummary[]>([])
const summaryPagination = reactive({ page: 1, pageSize: 20, total: 0 })

// Task configs
const taskConfigLoading = ref(false)
const taskConfigs = ref<TaskConfig[]>([])

// Exchange config
const exchangeConfig = reactive({
  auto_update_products: false,
  concurrency: 10,
  enabled: true,
  exchange_monthly_enabled: false,
  exchange_time: '10:00',
  monthly_prize_id: '1001',
  immediate_exchange_enabled: false
})
const updateProductsLoading = ref(false)
const monthlyExchangeLoading = ref(false)
const selectedAccountId = ref<number | null>(null)
const allAccounts = ref<Account[]>([])
const availableProductSourceAccounts = computed(() =>
  allAccounts.value.filter(account => account.is_active)
)

// User management
const userLoading = ref(false)
const userList = ref<User[]>([])
const userPagination = reactive({ page: 1, pageSize: 10, total: 0 })

// Stats
const statsOverview = ref([
  { key: 'user_count', label: '用户总数', value: 0 as any, icon: 'User', color: 'linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%)' },
  { key: 'account_count', label: '账号总数', value: 0 as any, icon: 'Document', color: 'linear-gradient(135deg, #10b981 0%, #34d399 100%)' },
  { key: 'total_cloud', label: '总云朵数', value: 0 as any, icon: 'Cloudy', color: 'linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%)' },
  { key: 'active_tasks', label: '活跃任务', value: 0 as any, icon: 'TrendCharts', color: 'linear-gradient(135deg, #ef4444 0%, #f87171 100%)' }
])

// Role dialog
const roleDialogVisible = ref(false)
const roleForm = reactive({ id: 0, role: 'user' })
const passwordDialogVisible = ref(false)
const passwordForm = reactive({ id: 0, username: '', password: '' })

// Announcements
const announcementLoading = ref(false)
const announcements = ref<Announcement[]>([])
const announcementDialogVisible = ref(false)
const viewAnnouncementVisible = ref(false)
const isEditingAnnouncement = ref(false)
const announcementSubmitting = ref(false)
const currentAnnouncement = ref<Announcement | null>(null)
const announcementFormRef = ref()
const announcementForm = reactive({
  id: 0,
  title: '',
  content: '',
  is_popup: false,
  is_top: false,
  is_published: true
})
const announcementRules = {
  title: [{ required: true, message: '请输入公告标题', trigger: 'blur' }],
  content: [{ required: true, message: '请输入公告内容', trigger: 'blur' }]
}

const handleTabChange = (tab: string) => {
  if (tab === 'summaries') loadSummaries()
  else if (tab === 'tasks') loadTaskConfigs()
  else if (tab === 'exchange') loadExchangeConfig()
  else if (tab === 'users') loadUserList()
  else if (tab === 'stats') loadStatsOverview()
  else if (tab === 'announcements') loadAnnouncements()
}

// Load account summaries
const loadSummaries = async () => {
  summaryLoading.value = true
  try {
    const data = await getAccountSummaries(summaryPagination.page, summaryPagination.pageSize)
    summaryList.value = data.summaries || []
    summaryPagination.total = data.total
  } catch { ElMessage.error('加载账号概况失败') }
  finally { summaryLoading.value = false }
}

// Load task configs
const loadTaskConfigs = async () => {
  taskConfigLoading.value = true
  try {
    const data = await getTaskConfigs()
    taskConfigs.value = data.configs || []
  } catch { ElMessage.error('加载任务配置失败') }
  finally { taskConfigLoading.value = false }
}

const handleTaskConfigChange = async (row: TaskConfig) => {
  try {
    await updateTaskConfig(row.task_type, row.is_enabled)
    ElMessage.success(row.is_enabled ? '任务已上架' : '任务已下架')
  } catch {
    row.is_enabled = !row.is_enabled
    ElMessage.error('操作失败')
  }
}

// Load exchange config
const loadExchangeConfig = async () => {
  try {
    const data = await getExchangeConfig()
    exchangeConfig.auto_update_products = data.auto_update_products
    exchangeConfig.concurrency = data.concurrency
    exchangeConfig.enabled = data.enabled
    exchangeConfig.exchange_monthly_enabled = data.exchange_monthly_enabled || false
    exchangeConfig.exchange_time = data.exchange_time || '10:00'
    exchangeConfig.monthly_prize_id = data.monthly_prize_id || '1001'
    exchangeConfig.immediate_exchange_enabled = data.immediate_exchange_enabled || false
    
    // 加载所有账号用于商品更新
    const accountsData = await getAllAccounts(1, 1000)
    console.log('获取到的账号数据:', accountsData)
    allAccounts.value = accountsData.accounts || []
    console.log('加载账号数量:', allAccounts.value.length)

    const hasSelectedAvailableAccount = availableProductSourceAccounts.value.some(
      account => account.id === selectedAccountId.value
    )
    if (!hasSelectedAvailableAccount) {
      selectedAccountId.value = availableProductSourceAccounts.value[0]?.id ?? null
    }
  } catch (error: any) {
    ElMessage.error('加载抢兑配置失败：' + error.message)
  }
}

// Save exchange config
const saveExchangeConfig = async () => {
  try {
    await updateExchangeConfig({
      auto_update_products: exchangeConfig.auto_update_products,
      concurrency: exchangeConfig.concurrency,
      enabled: exchangeConfig.enabled,
      exchange_monthly_enabled: exchangeConfig.exchange_monthly_enabled,
      exchange_time: exchangeConfig.exchange_time,
      monthly_prize_id: exchangeConfig.monthly_prize_id,
      immediate_exchange_enabled: exchangeConfig.immediate_exchange_enabled
    })
    ElMessage.success('保存配置成功')
  } catch (error: any) {
    ElMessage.error('保存配置失败：' + error.message)
  }
}

// Update products
const handleUpdateProducts = async () => {
  if (!selectedAccountId.value) {
    ElMessage.warning('请先选择一个云盘账号')
    return
  }
  updateProductsLoading.value = true
  try {
    await apiUpdateProducts(selectedAccountId.value)
    ElMessage.success('商品数据更新成功')
  } catch (error: any) {
    ElMessage.error('更新商品数据失败：' + error.message)
  } finally {
    updateProductsLoading.value = false
  }
}

// Execute monthly exchange
const executeMonthlyExchange = async () => {
  monthlyExchangeLoading.value = true
  try {
    await apiExecuteMonthlyExchange()
    ElMessage.success('已开始执行兑换月卡任务')
  } catch (error: any) {
    ElMessage.error('执行兑换月卡失败：' + error.message)
  } finally {
    monthlyExchangeLoading.value = false
  }
}

// Load users
const loadUserList = async () => {
  userLoading.value = true
  try {
    const data = await getAllUsers(userPagination.page, userPagination.pageSize)
    userList.value = data.users as any[]
    userPagination.total = data.total
  } catch { ElMessage.error('加载用户列表失败') }
  finally { userLoading.value = false }
}

const handleEditUserRole = (row: User) => {
  roleForm.id = row.id
  roleForm.role = row.role
  roleDialogVisible.value = true
}

const handleRoleSubmit = async () => {
  try {
    await updateUserRole(roleForm.id, roleForm.role)
    ElMessage.success('角色修改成功')
    roleDialogVisible.value = false
    loadUserList()
  } catch { ElMessage.error('角色修改失败') }
}

const handleResetUserPassword = (row: User) => {
  passwordForm.id = row.id
  passwordForm.username = row.username
  passwordForm.password = ''
  passwordDialogVisible.value = true
}

const handlePasswordSubmit = async () => {
  if (passwordForm.password.length < 12) {
    ElMessage.warning('新密码长度不能少于12个字符')
    return
  }
  try {
    await resetUserPassword(passwordForm.id, passwordForm.password)
    ElMessage.success('密码重置成功')
    passwordDialogVisible.value = false
  } catch (error: any) {
    ElMessage.error(error.response?.data?.message || '密码重置失败')
  }
}

const handleDeleteUser = async (row: User) => {
  try {
    await ElMessageBox.confirm('确定要删除该用户吗？', '提示', { type: 'warning' })
    await deleteUser(row.id)
    ElMessage.success('删除成功')
    loadUserList()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error('删除失败') }
}

// Load stats
const loadStatsOverview = async () => {
  try {
    const data = await getStatsOverview()
    statsOverview.value[0].value = data.user_count
    statsOverview.value[1].value = data.account_count
    statsOverview.value[2].value = data.total_cloud
    statsOverview.value[3].value = data.active_tasks
  } catch { console.error('加载统计概览失败') }
}

const formatDate = (date: string | undefined) => {
  if (!date) return ''
  return new Date(date).toLocaleString('zh-CN')
}

// Announcement methods
const loadAnnouncements = async () => {
  announcementLoading.value = true
  try {
    const res: any = await getAllAnnouncements()
    announcements.value = res.announcements || []
  } catch (error: any) {
    ElMessage.error('加载公告失败：' + error.message)
  } finally {
    announcementLoading.value = false
  }
}

const showAddAnnouncementDialog = () => {
  isEditingAnnouncement.value = false
  announcementForm.id = 0
  announcementForm.title = ''
  announcementForm.content = ''
  announcementForm.is_popup = false
  announcementForm.is_top = false
  announcementForm.is_published = true
  announcementDialogVisible.value = true
}

const editAnnouncement = (row: Announcement) => {
  isEditingAnnouncement.value = true
  announcementForm.id = row.id
  announcementForm.title = row.title
  announcementForm.content = row.content
  announcementForm.is_popup = row.is_popup
  announcementForm.is_top = row.is_top
  announcementForm.is_published = row.is_published
  announcementDialogVisible.value = true
}

const viewAnnouncement = (row: Announcement) => {
  currentAnnouncement.value = row
  viewAnnouncementVisible.value = true
}

const submitAnnouncementForm = async () => {
  const valid = await announcementFormRef.value?.validate().catch(() => false)
  if (!valid) return

  announcementSubmitting.value = true
  try {
    if (isEditingAnnouncement.value) {
      const data: UpdateAnnouncementRequest = {
        title: announcementForm.title,
        content: announcementForm.content,
        is_popup: announcementForm.is_popup,
        is_top: announcementForm.is_top,
        is_published: announcementForm.is_published
      }
      await updateAnnouncement(announcementForm.id, data)
      ElMessage.success('更新成功')
    } else {
      const data: CreateAnnouncementRequest = {
        title: announcementForm.title,
        content: announcementForm.content,
        is_popup: announcementForm.is_popup,
        is_top: announcementForm.is_top
      }
      await createAnnouncement(data)
      ElMessage.success('发布成功')
    }
    announcementDialogVisible.value = false
    loadAnnouncements()
  } catch (error: any) {
    ElMessage.error(isEditingAnnouncement.value ? '更新失败：' : '发布失败：' + error.message)
  } finally {
    announcementSubmitting.value = false
  }
}

const deleteAnnouncement = async (row: Announcement) => {
  try {
    await ElMessageBox.confirm('确定要删除这条公告吗？', '确认删除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await apiDeleteAnnouncement(row.id)
    ElMessage.success('删除成功')
    loadAnnouncements()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
  loadSummaries()
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewport)
})
</script>

<style scoped>
.admin-panel-container { padding: clamp(12px, 2vw, 24px); max-width: 1680px; margin: 0 auto; }
.admin-shell { overflow: hidden; }
:deep(.el-card) { border-radius: 24px; box-shadow: 0 16px 42px rgba(37, 99, 235, 0.1); border: 1px solid rgba(255,255,255,.74); background: rgba(255,255,255,.88); backdrop-filter: blur(14px); }
.card-header { display:flex; justify-content:space-between; align-items:center; gap:12px; font-size:clamp(20px,2.4vw,26px); font-weight:700; color:#0f172a; }
.admin-tabs { margin-top: 6px; }
.admin-tabs :deep(.el-tabs__header) { margin:0; padding-bottom:10px; }
.admin-tabs :deep(.el-tabs__nav-wrap) { overflow-x:auto; scrollbar-width:none; }
.admin-tabs :deep(.el-tabs__nav-wrap::-webkit-scrollbar) { display:none; }
.admin-tabs :deep(.el-tabs__nav-scroll) { display:flex; }
.admin-tabs :deep(.el-tabs__nav) { flex-wrap:nowrap; }
.admin-tabs :deep(.el-tabs__item) { height:44px; padding:0 18px; font-size:15px; font-weight:600; white-space:nowrap; }
.admin-tabs :deep(.el-tabs__item.is-active) { color:#2563eb; }
.admin-tabs :deep(.el-tabs__active-bar) { background: linear-gradient(90deg, #2563eb, #0ea5e9); }
.tab-content { padding: 22px 0 0; display:flex; flex-direction:column; gap:20px; }
.tab-content > p { margin:0; line-height:1.6; }
.config-card { height:100%; border-radius:22px; }
.config-header { display:flex; justify-content:space-between; align-items:center; gap:12px; font-size:16px; font-weight:700; color:#0f172a; }
.product-management { padding-top: 4px; }
.product-management :deep(.el-form--inline) { display:flex; flex-wrap:wrap; gap:12px 16px; align-items:flex-end; }
.product-management :deep(.el-form-item) { margin:0; }
.product-management :deep(.el-form-item__content) { width:100%; }
.product-management :deep(.el-select), .product-management :deep(.el-input) { width:min(100%, 280px)!important; }
.stat-card { height:100%; margin-bottom:0; }
.stat-content { display:flex; align-items:center; gap:14px; }
.stat-icon { width:54px; height:54px; border-radius:16px; display:flex; align-items:center; justify-content:center; flex-shrink:0; box-shadow:0 10px 20px rgba(15,23,42,.12); }
.stat-icon .el-icon { font-size:26px; color:#fff; }
.stat-info { min-width:0; }
.stat-info .stat-value { font-size:clamp(24px,2.4vw,30px); font-weight:800; color:#0f172a; }
.stat-info .stat-label { margin-top:4px; font-size:13px; color:#64748b; }
.task-status-cell { display:inline-flex; align-items:center; gap:8px; flex-wrap:wrap; }
.status-on { color:#10b981; font-size:13px; font-weight:600; }
.status-off { color:#ef4444; font-size:13px; font-weight:600; }
.announcement-content { padding-top:10px; gap:10px; }
.announcement-header { margin-bottom:0; min-height:34px; display:flex; justify-content:flex-end; align-items:center; }
.title-cell { display:flex; align-items:center; gap:8px; min-width:0; }
.title-text { flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.ml-2 { margin-left:8px; }
.form-options { display:flex; gap:14px; flex-wrap:wrap; }
.tip-item { margin-bottom:0; }
.view-content { padding:10px 4px 4px; }
.view-title { font-size:20px; font-weight:700; color:#0f172a; margin:0 0 16px; line-height:1.45; }
.view-meta { display:flex; align-items:center; gap:10px; flex-wrap:wrap; margin-bottom:20px; padding-bottom:14px; border-bottom:1px solid rgba(148,163,184,.2); }
.view-time { color:#64748b; font-size:13px; }
.view-body { font-size:14px; line-height:1.8; color:#475569; white-space:pre-wrap; }
:deep(.el-table) { border-radius:18px; overflow:hidden; --el-table-border-color: rgba(148,163,184,.18); --el-table-header-bg-color: rgba(248,250,252,.9); --el-table-row-hover-bg-color: rgba(239,246,255,.72); }
:deep(.el-table .cell) { line-height:1.45; }
:deep(.el-table th.el-table__cell) { color:#475569; font-size:13px; font-weight:700; }
:deep(.el-table td.el-table__cell) { color:#334155; }
:deep(.el-pagination) { justify-content:flex-end; flex-wrap:wrap; gap:8px; }
:deep(.el-dialog) { max-width: calc(100vw - 32px); border-radius:22px; }
@media (max-width: 1280px) { .admin-panel-container { padding:14px; } .tab-content { gap:16px; padding-top:18px; } :deep(.el-col-12) { width:100%!important; max-width:100%!important; flex:0 0 100%!important; } :deep(.el-col-6) { width:50%!important; max-width:50%!important; flex:0 0 50%!important; } }
@media (max-width: 768px) { .admin-panel-container { padding:0; } .card-header { font-size:20px; } .admin-tabs :deep(.el-tabs__item) { height:40px; padding:0 14px; font-size:13px; } .tab-content { padding-top:16px; gap:14px; } .announcement-content { padding-top:10px; gap:10px; } .announcement-header { justify-content:stretch; min-height:34px; } .announcement-header :deep(.el-button) { width:100%; } .product-management :deep(.el-form--inline) { flex-direction:column; align-items:stretch; } .product-management :deep(.el-select), .product-management :deep(.el-input), .product-management :deep(.el-button) { width:100%!important; } .task-status-cell { align-items:flex-start; } .form-options { flex-direction:column; gap:10px; } .view-title { font-size:18px; } :deep(.el-col-6) { width:50%!important; max-width:50%!important; flex:0 0 50%!important; } :deep(.el-pagination) { justify-content:center; } }
@media (max-width: 520px) { :deep(.el-col-6) { width:100%!important; max-width:100%!important; flex:0 0 100%!important; } }

.responsive-data-shell { min-height: 120px; }
.mobile-admin-list { display: grid; gap: 12px; }
.mobile-admin-card { border-radius: 18px; border: 1px solid rgba(226, 232, 240, 0.9); background: rgba(255, 255, 255, 0.94); box-shadow: 0 14px 28px rgba(37, 99, 235, 0.08); }
.mobile-admin-card :deep(.el-card__body) { padding: 16px; }
.mobile-admin-card-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.mobile-admin-card-title { font-size: 15px; font-weight: 700; color: #0f172a; line-height: 1.4; word-break: break-word; }
.mobile-admin-card-meta { margin-top: 4px; font-size: 12px; color: #64748b; }
.mobile-admin-card-grid { display: grid; gap: 10px; }
.mobile-admin-card-row { display: grid; grid-template-columns: 84px minmax(0, 1fr); gap: 10px; align-items: start; }
.mobile-admin-card-row.full { grid-template-columns: 1fr; }
.mobile-admin-card-label { font-size: 12px; color: #64748b; font-weight: 600; }
.mobile-admin-card-value { font-size: 13px; color: #334155; word-break: break-word; }
.mobile-admin-card-value.strong { font-weight: 700; color: #2563eb; }
.mobile-admin-card-value.success { color: #059669; font-weight: 600; }
.mobile-admin-card-value.multiline { line-height: 1.6; }
.mobile-admin-inline-tags { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.mobile-admin-card-footer { margin-top: 14px; padding-top: 12px; border-top: 1px solid rgba(226, 232, 240, 0.8); }
.mobile-admin-card-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
.mobile-admin-card-actions :deep(.el-button) { flex: 1 1 120px; margin: 0; }
@media (max-width: 768px) {
  .responsive-data-shell :deep(.el-table) { display: none; }
  .mobile-admin-card-row { grid-template-columns: 78px minmax(0, 1fr); }
}
</style>

