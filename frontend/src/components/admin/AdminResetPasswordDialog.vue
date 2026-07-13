<template>
  <el-dialog
    v-model="visible"
    title="重置用户密码"
    width="420px"
  >
    <el-form
      :model="form"
      label-width="90px"
    >
      <el-form-item label="用户">
        <el-input
          v-model="form.username"
          disabled
        />
      </el-form-item>
      <el-form-item label="新密码">
        <el-input
          v-model="form.password"
          type="password"
          show-password
          placeholder="至少12位，包含至少三类字符"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">
        取消
      </el-button>
      <el-button
        type="primary"
        @click="$emit('submit')"
      >
        确定重置
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
const visible = defineModel<boolean>({ required: true })
const form = defineModel<{ id: number; username: string; password: string }>('form', { required: true })

defineEmits<{
  submit: []
}>()
</script>
