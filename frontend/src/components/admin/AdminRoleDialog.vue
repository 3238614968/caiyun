<template>
  <el-dialog
    v-model="visible"
    title="修改用户角色"
    width="400px"
  >
    <el-form
      :model="form"
      label-width="80px"
    >
      <el-form-item label="角色">
        <el-radio-group v-model="form.role">
          <el-radio label="user">
            普通用户
          </el-radio>
          <el-radio label="admin">
            管理员
          </el-radio>
        </el-radio-group>
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
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
const visible = defineModel<boolean>({ required: true })
const form = defineModel<{ id: number; role: string }>('form', { required: true })

defineEmits<{
  submit: []
}>()
</script>
