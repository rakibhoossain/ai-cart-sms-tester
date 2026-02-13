"use client"

import { useEffect, useState, useRef } from "react"
import { useMessageStore, SMS } from "@/lib/store"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Trash2, Search, Mail, RefreshCcw, Send } from "lucide-react"
import { format } from "date-fns"

import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"

export default function Dashboard() {
  const { messages, pagination, search, setSearch, fetchMessages, connectWebSocket, clearMessages, addMessage, markAsRead } = useMessageStore()
  const [selectedMessage, setSelectedMessage] = useState<SMS | null>(null)
  const [isSimOpen, setIsSimOpen] = useState(false)

  // Simulation Simulation State
  const [simTo, setSimTo] = useState("")
  const [simBody, setSimBody] = useState("")

  useEffect(() => {
    fetchMessages()
    connectWebSocket()
  }, [])

  // Debounced Search
  useEffect(() => {
    const delayDebounceFn = setTimeout(() => {
      fetchMessages(1, false)
    }, 300)

    return () => clearTimeout(delayDebounceFn)
  }, [search, fetchMessages])

  const observerTarget = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const observer = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting) {
          const nextPage = pagination.page + 1
          const maxPage = Math.ceil(pagination.total / pagination.limit) || 1
          if (nextPage <= maxPage) {
            fetchMessages(nextPage, true)
          }
        }
      },
      { threshold: 0.1 }
    )

    if (observerTarget.current) {
      observer.observe(observerTarget.current)
    }

    return () => {
      if (observerTarget.current) {
        observer.unobserve(observerTarget.current)
      }
    }
  }, [pagination, fetchMessages])

  // Client-side filtering removed in favor of backend search
  const filteredMessages = messages

  const handleClear = async () => {
    if (confirm("Are you sure you want to delete all messages?")) {
      await fetch('/api/v1/messages', { method: 'DELETE' })
      clearMessages()
      setSelectedMessage(null)
    }
  }

  const handleSimulateSend = async () => {
    if (!simTo || !simBody) return

    await fetch('/api/v1/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        from: "+15550199",
        to: simTo,
        body: simBody
      })
    })
    setSimTo("")
    setSimBody("")
    setIsSimOpen(false)
  }

  const handleSelectMessage = (msg: SMS) => {
    setSelectedMessage(msg)
    if (!msg.is_read) {
      markAsRead(msg.id)
    }
  }

  return (
    <div className="h-screen flex flex-col bg-background text-foreground">
      {/* Header */}
      <header className="border-b p-4 flex flex-col md:flex-row items-start md:items-center justify-between gap-4 bg-card">
        <div className="flex items-center justify-between w-full md:w-auto gap-4">
          <div className="flex items-center gap-2">
            <Mail className="h-6 w-6 text-primary" />
            <h1 className="text-xl font-bold">SMS Tester</h1>
          </div>
          <div className="flex gap-2">
            <Badge variant="secondary" className="text-xs">Total: {pagination.total}</Badge>
            <Badge variant={pagination.unread > 0 ? "destructive" : "secondary"} className="text-xs">Unread: {pagination.unread}</Badge>
          </div>
        </div>

        <div className="flex items-center gap-2 w-full md:w-auto justify-between md:justify-end">
          <Dialog open={isSimOpen} onOpenChange={setIsSimOpen}>
            <DialogTrigger asChild>
              <Button variant="default" size="sm" title="Simulate SMS">
                <Send className="mr-2 h-4 w-4" /> <span className="sr-only md:not-sr-only">Simulate</span>
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Simulate Incoming SMS</DialogTitle>
                <DialogDescription>
                  Send a test message to the system.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-4">
                <div className="space-y-2">
                  <Input placeholder="To Phone Number" value={simTo} onChange={e => setSimTo(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Textarea
                    placeholder="Message Body (supports emojis 🚀)"
                    value={simBody}
                    onChange={e => setSimBody(e.target.value)}
                    className="min-h-[100px]"
                  />
                </div>
              </div>
              <DialogFooter>
                <Button onClick={handleSimulateSend} disabled={!simTo || !simBody}>Send Message</Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>

          <div className="relative flex-1 md:w-64">
            <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search..."
              className="pl-8"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Button variant="outline" size="icon" onClick={() => fetchMessages(pagination.page)}>
            <RefreshCcw className="h-4 w-4" />
          </Button>
          <Button variant="destructive" size="icon" onClick={handleClear}>
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </header>

      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar List */}
        <div className="w-1/3 border-r flex flex-col bg-muted/10">
          <ScrollArea className="flex-1 h-full">
            <div className="flex flex-col">
              {filteredMessages.length === 0 ? (
                <div className="p-8 text-center text-muted-foreground">
                  No messages found
                </div>
              ) : (
                <>
                  {filteredMessages.map((msg) => (
                    <button
                      key={msg.id}
                      className={`flex flex-col items-start gap-1 p-4 border-b text-left hover:bg-muted/50 transition-colors ${selectedMessage?.id === msg.id ? "bg-muted" : ""} ${!msg.is_read ? "bg-primary/5" : ""}`}
                      onClick={() => handleSelectMessage(msg)}
                    >
                      <div className="flex w-full flex-col gap-1">
                        <div className="flex items-center">
                          <div className="flex items-center gap-2">
                            {!msg.is_read && <div className="w-2 h-2 rounded-full bg-blue-500" />}
                            <div className={`font-semibold ${!msg.is_read ? "text-foreground" : "text-muted-foreground"}`}>{msg.from}</div>
                          </div>
                          <div className="ml-auto text-xs text-muted-foreground">
                            {format(new Date(msg.created_at), "HH:mm:ss")}
                          </div>
                        </div>
                        <div className="text-xs font-medium">{msg.to}</div>
                      </div>
                      <div className={`line-clamp-2 text-xs ${!msg.is_read ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                        {msg.body.substring(0, 300)}
                      </div>
                    </button>
                  ))}
                  <div ref={observerTarget} className="p-4 text-center text-xs text-muted-foreground">
                    {pagination.page < (Math.ceil(pagination.total / pagination.limit) || 1) ? 'Loading more...' : 'No more messages'}
                  </div>
                </>
              )}
            </div>
          </ScrollArea>
        </div>

        {/* Detail View */}
        <div className="flex-1 p-6 flex flex-col overflow-auto">
          {selectedMessage ? (
            <Card className="flex-1 flex flex-col shadow-none border-0">
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div className="space-y-1">
                    <CardTitle>Message Details</CardTitle>
                    <CardDescription>ID: {selectedMessage.id}</CardDescription>
                  </div>
                  <Badge variant="outline">
                    {format(new Date(selectedMessage.created_at), "PPpp")}
                  </Badge>
                </div>
              </CardHeader>
              <Separator />
              <CardContent className="flex-1 pt-6 space-y-6">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-1">
                    <span className="text-sm font-medium text-muted-foreground">From</span>
                    <div className="text-lg font-mono bg-muted/30 p-2 rounded">{selectedMessage.from}</div>
                  </div>
                  <div className="space-y-1">
                    <span className="text-sm font-medium text-muted-foreground">To</span>
                    <div className="text-lg font-mono bg-muted/30 p-2 rounded">{selectedMessage.to}</div>
                  </div>
                </div>

                <div className="space-y-2">
                  <span className="text-sm font-medium text-muted-foreground">Body</span>
                  <div className="p-4 bg-muted/20 rounded-lg min-h-[200px] whitespace-pre-wrap font-sans text-base">
                    {selectedMessage.body}
                  </div>
                </div>

                <div className="space-y-2">
                  <span className="text-sm font-medium text-muted-foreground">Raw JSON</span>
                  <pre className="p-4 bg-muted/50 rounded-lg text-xs overflow-auto">
                    {JSON.stringify(selectedMessage, null, 2)}
                  </pre>
                </div>
              </CardContent>
            </Card>
          ) : (
            <div className="flex-1 flex items-center justify-center text-muted-foreground">
              Select a message to view details
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
