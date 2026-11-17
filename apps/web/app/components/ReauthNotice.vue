<script setup lang="ts">
import { authMessages } from '../../i18n/auth'
import type { Language } from '../../i18n'
import type { ReauthAction } from '../utils/reauth-pending'

const props = defineProps<{ action: ReauthAction }>()
defineEmits<{ signIn: [], cancel: [] }>()
const language = useState<Language>('language', () => 'vi')
const labels = computed(() => authMessages(language.value))
const message = computed(() => props.action === 'token' ? labels.value.reauthToken : props.action === 'password-link' ? labels.value.reauthPasswordLink : labels.value.reauthAuthenticator)
</script>
<template>
  <div role="status"><p>{{ message }}</p><div class="auth-actions"><button class="btn btn-primary" type="button" @click="$emit('signIn')">{{ labels.signInAgain }}</button><button class="btn btn-secondary" type="button" @click="$emit('cancel')">{{ labels.cancel }}</button></div></div>
</template>
