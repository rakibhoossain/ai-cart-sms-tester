import { create } from 'zustand'

export type SMS = {
  id: number
  from: string
  to: string
  body: string
  is_read: boolean
  created_at: string
}

type PaginationState = {
  page: number
  limit: number
  total: number
  unread: number
}

type MessageStore = {
  messages: SMS[]
  pagination: PaginationState
  search: string
  setSearch: (search: string) => void
  setMessages: (messages: SMS[]) => void
  addMessage: (message: SMS) => void
  clearMessages: () => void
  fetchMessages: (page?: number, append?: boolean) => Promise<void>
  markAsRead: (id: number) => Promise<void>
  connectWebSocket: () => void
}

export const useMessageStore = create<MessageStore>((set, get) => ({
  messages: [],
  pagination: { page: 1, limit: 50, total: 0, unread: 0 },
  search: '',
  setSearch: (search) => set({ search }),
  setMessages: (messages) => set({ messages }),
  addMessage: (message) => set((state) => ({
    messages: [message, ...state.messages],
    pagination: { ...state.pagination, total: state.pagination.total + 1, unread: state.pagination.unread + 1 }
  })),
  clearMessages: () => set({ messages: [], pagination: { page: 1, limit: 50, total: 0, unread: 0 } }),
  fetchMessages: async (page = 1, append = false) => {
    try {
      const search = get().search
      const query = new URLSearchParams({
        page: page.toString(),
        limit: '50',
        search: search || ''
      })
      const res = await fetch(`/api/v1/messages?${query.toString()}`)
      if (res.ok) {
        const json = await res.json()
        set((state) => {
          const newMessages = json.data || []

          if (!append) {
            return {
              messages: newMessages,
              pagination: {
                page: json.meta.page,
                limit: json.meta.limit,
                total: json.meta.total,
                unread: json.meta.unread
              }
            }
          }

          // Deduplicate: Filter out messages that already exist in state
          const existingIds = new Set(state.messages.map(m => m.id))
          const uniqueNewMessages = newMessages.filter((m: SMS) => !existingIds.has(m.id))

          return {
            messages: [...state.messages, ...uniqueNewMessages],
            pagination: {
              page: json.meta.page,
              limit: json.meta.limit,
              total: json.meta.total,
              unread: json.meta.unread
            }
          }
        })
      }
    } catch (error) {
      console.error('Failed to fetch messages:', error)
    }
  },
  markAsRead: async (id: number) => {
    try {
      await fetch(`/api/v1/messages/${id}/read`, { method: 'PUT' })
      set((state) => ({
        messages: state.messages.map(m => m.id === id ? { ...m, is_read: true } : m),
        pagination: { ...state.pagination, unread: Math.max(0, state.pagination.unread - 1) }
      }))
    } catch (error) {
      console.error('Failed to mark as read:', error)
    }
  },
  connectWebSocket: () => {
    if (typeof window === 'undefined') return
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws`
    const ws = new WebSocket(wsUrl)
    ws.onmessage = (event) => {
      const message = JSON.parse(event.data)
      get().addMessage(message)
    }
    ws.onclose = () => {
      setTimeout(() => get().connectWebSocket(), 3000)
    }
  },
}))
