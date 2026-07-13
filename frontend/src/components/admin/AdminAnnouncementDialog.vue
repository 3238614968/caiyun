<template>
  <el-dialog
    v-model="visible"
    :title="isEditing ? '编辑公告' : '发布公告'"
    width="700px"
  >
    <el-form
      ref="formRef"
      :model="form"
      label-position="top"
      :rules="rules"
    >
      <el-form-item
        label="公告标题"
        prop="title"
      >
        <el-input
          v-model="form.title"
          placeholder="请输入公告标题"
          maxlength="100"
          show-word-limit
        />
      </el-form-item>
      <el-form-item
        label="公告内容"
        prop="content"
      >
        <el-input
          v-model="form.content"
          type="textarea"
          :rows="6"
          placeholder="请输入公告内容"
          maxlength="2000"
          show-word-limit
        />
      </el-form-item>
      <el-form-item>
        <div class="form-options">
          <el-checkbox
            v-model="form.is_popup"
            label="弹窗显示"
            border
          />
          <el-checkbox
            v-model="form.is_top"
            label="置顶"
            border
          />
          <el-checkbox
            v-if="isEditing"
            v-model="form.is_published"
            label="发布状态"
            border
          />
        </div>
      </el-form-item>
      <el-form-item
        v-if="form.is_popup"
        class="tip-item"
      >
        <el-alert
          title="弹窗公告说明"
          type="info"
          :closable="false"
          description="开启弹窗后，仅置顶且未读的弹窗公告会自动弹出一次；其他已发布公告会展示在首页公告列表中。"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">
        取消
      </el-button>
      <el-button
        type="primary"
        :loading="loading"
        @click="handleSubmit"
      >
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { FormRules } from 'element-plus'

const visible = defineModel<boolean>({ required: true })
const form = defineModel<{
  id: number
  title: string
  content: string
  is_popup: boolean
  is_top: boolean
  is_published: boolean
}>('form', { required: true })

defineProps<{
  isEditing: boolean
  loading: boolean
  rules: FormRules
}>()

const emit = defineEmits<{
  submit: []
}>()

const formRef = ref()

const handleSubmit = async () => {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  emit('submit')
}
</script>

<style scoped>
.form-options { display:flex; gap:14px; flex-wrap:wrap; }
.tip-item { margin-bottom:0; }
@media (max-width: 768px) {
  .form-options { flex-direction:column; gap:10px; }
}
</style>
