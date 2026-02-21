import { useRef, useEffect, useState, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Send, Loader2, MessageSquare } from 'lucide-react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import type { ChatMessage, AgentInstance } from '@/types/agent-canvas'

interface AgentChatViewProps {
  agent: AgentInstance
  messages: ChatMessage[]
  isStreaming?: boolean
  onSendMessage: (content: string) => void
}

function ChatBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === 'user'
  const isSystem = message.role === 'system'

  if (isSystem) {
    return (
      <div className="flex justify-center px-4 py-1">
        <span className="text-[11px] text-muted-foreground italic">
          {message.content}
        </span>
      </div>
    )
  }

  return (
    <div
      className={cn(
        'flex w-full px-3 py-1',
        isUser ? 'justify-end' : 'justify-start'
      )}
    >
      <div
        className={cn(
          'max-w-[85%] rounded-lg px-3 py-2 text-sm leading-relaxed',
          isUser
            ? 'bg-primary text-primary-foreground rounded-br-sm'
            : 'bg-muted/60 text-foreground border border-border/40 rounded-bl-sm'
        )}
      >
        {isUser ? (
          <p className="whitespace-pre-wrap break-words">{message.content}</p>
        ) : (
          <div className="prose prose-sm prose-invert max-w-none break-words [&_p]:my-1 [&_pre]:my-2 [&_pre]:rounded-md [&_pre]:bg-background/60 [&_pre]:p-2 [&_code]:text-xs [&_ul]:my-1 [&_ol]:my-1 [&_li]:my-0.5">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {message.content}
            </ReactMarkdown>
          </div>
        )}
        <div
          className={cn(
            'mt-1 text-[10px] opacity-60',
            isUser ? 'text-right' : 'text-left'
          )}
        >
          {new Date(message.timestamp).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
          })}
        </div>
      </div>
    </div>
  )
}

export function AgentChatView({
  agent,
  messages,
  isStreaming = false,
  onSendMessage,
}: AgentChatViewProps) {
  const [input, setInput] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages.length])

  const handleSend = useCallback(() => {
    const trimmed = input.trim()
    if (!trimmed || isStreaming) return
    onSendMessage(trimmed)
    setInput('')
    inputRef.current?.focus()
  }, [input, isStreaming, onSendMessage])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        handleSend()
      }
    },
    [handleSend]
  )

  const isEmpty = messages.length === 0

  return (
    <div className="flex h-full flex-col">
      {/* Messages area */}
      <ScrollArea className="flex-1 min-h-0">
        <div ref={scrollRef} className="flex flex-col gap-2 py-2">
          {isEmpty ? (
            <div className="flex flex-col items-center justify-center gap-3 py-12 text-muted-foreground">
              <MessageSquare className="h-8 w-8 opacity-40" />
              <p className="text-sm">Start a conversation with {agent.name}</p>
            </div>
          ) : (
            messages.map((msg) => <ChatBubble key={msg.id} message={msg} />)
          )}

          {/* Streaming indicator */}
          {isStreaming && (
            <div className="flex justify-start px-3 py-1">
              <div className="flex items-center gap-2 rounded-lg bg-muted/60 border border-border/40 px-3 py-2 text-sm text-muted-foreground">
                <Loader2 className="h-3 w-3 animate-spin" />
                <span>Thinking...</span>
              </div>
            </div>
          )}
        </div>
      </ScrollArea>

      {/* Input bar */}
      <div className="border-t border-border/60 p-2">
        <div className="flex items-center gap-2">
          <Input
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={`Message ${agent.name}...`}
            disabled={isStreaming || agent.status === 'error'}
            className="h-8 text-sm bg-background/60"
          />
          <Button
            size="icon-sm"
            variant="default"
            onClick={handleSend}
            disabled={!input.trim() || isStreaming || agent.status === 'error'}
            aria-label="Send message"
          >
            <Send className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </div>
  )
}
