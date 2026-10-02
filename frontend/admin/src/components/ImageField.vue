<script setup lang="ts">
import { ref } from 'vue'
import { uploadImage, type UploadScope } from '../api/uploads'
import { errorMessage } from '../utils/format'

const model = defineModel<string>({ default: '' })

const props = withDefaults(
  defineProps<{
    label?: string
    // The id of the text input, so a caller's label can point at it.
    inputId?: string
    placeholder?: string
    scope?: UploadScope
    // Whether to show a small live picture of the value. A form with its own
    // big preview leaves this off so the same picture is not drawn twice.
    preview?: boolean
    // A reader of a screen it cannot save should not be offered a file to upload.
    disabled?: boolean
  }>(),
  {
    label: '',
    inputId: '',
    placeholder: 'https://… 或 /uploads/…',
    scope: 'products',
    preview: false,
    disabled: false,
  },
)

const file = ref<HTMLInputElement | null>(null)
const busy = ref(false)
const failure = ref('')

async function choose(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = input.files?.[0]
  // Cleared so that choosing the same file again after a refusal still counts as
  // a change, and so the field never shows a name it did not keep.
  input.value = ''
  if (!picked) return

  busy.value = true
  failure.value = ''
  try {
    model.value = (await uploadImage(picked, props.scope)).url
  } catch (err) {
    failure.value = errorMessage(err, '上传失败，请重试')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div>
    <label v-if="label" class="label" :for="inputId || undefined">{{ label }}</label>
    <div class="image-field flex items-center gap-2">
      <input v-model="model" :id="inputId || undefined" class="input min-w-0 mono text-xs" :placeholder="placeholder" />
      <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="busy || disabled" @click="file?.click()">
        {{ busy ? '上传中…' : '上传' }}
      </button>
    </div>
    <input ref="file" type="file" accept="image/png,image/jpeg,image/gif" class="hidden" @change="choose" />
    <p v-if="failure" class="mt-1 text-xs text-[var(--danger)]">{{ failure }}</p>
    <div v-else-if="preview && model" class="mt-2 flex items-center gap-2">
      <img :src="model" alt="" class="h-9 w-9 rounded-md border border-[var(--stroke)] object-cover" />
      <span class="hint">当前使用的图片</span>
    </div>
    <p v-else-if="preview" class="hint mt-2">留空则使用内置标识</p>
  </div>
</template>
