import client from './client'
import type { LoginResponse, User } from '../types'
export const login=(identifier:string,password:string)=>client.post<LoginResponse>('/auth/login',{identifier,password}).then(r=>r.data)
export const me=()=>client.get<{user:User}>('/auth/me').then(r=>r.data)
