import { createContext, useCallback, useContext, useMemo, useState } from "react"
import type { ReactNode } from "react"
import type { Space } from "@/types/space"

const SPACES_KEY = "hanzo.spaces"
const ACTIVE_KEY = "hanzo.activeSpaceId"

function now(): string {
  return new Date().toISOString()
}

function makeDefaultSpace(): Space {
  const ts = now()
  return {
    id: "default",
    name: "My Space",
    emoji: "\u{1F680}",
    createdAt: ts,
    updatedAt: ts,
  }
}

function loadSpaces(): Space[] {
  try {
    const raw = localStorage.getItem(SPACES_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Space[]
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed
      }
    }
  } catch {
    // corrupt data -- reset
  }
  const def = makeDefaultSpace()
  localStorage.setItem(SPACES_KEY, JSON.stringify([def]))
  return [def]
}

function loadActiveId(spaces: Space[]): string {
  try {
    const stored = localStorage.getItem(ACTIVE_KEY)
    if (stored && spaces.some((s) => s.id === stored)) {
      return stored
    }
  } catch {
    // ignore
  }
  const id = spaces[0]?.id ?? "default"
  localStorage.setItem(ACTIVE_KEY, id)
  return id
}

function persist(spaces: Space[], activeId: string) {
  localStorage.setItem(SPACES_KEY, JSON.stringify(spaces))
  localStorage.setItem(ACTIVE_KEY, activeId)
}

export interface SpaceContextValue {
  spaces: Space[]
  activeSpaceId: string | null
  activeSpace: Space | null
  createSpace: (name: string, emoji?: string) => Space
  deleteSpace: (id: string) => void
  renameSpace: (id: string, name: string) => void
  setActiveSpace: (id: string) => void
}

const SpaceContext = createContext<SpaceContextValue | undefined>(undefined)

export function SpaceProvider({ children }: { children: ReactNode }) {
  const [spaces, setSpaces] = useState<Space[]>(loadSpaces)
  const [activeSpaceId, setActiveSpaceId] = useState<string>(() => loadActiveId(loadSpaces()))

  const activeSpace = useMemo(
    () => spaces.find((s) => s.id === activeSpaceId) ?? null,
    [spaces, activeSpaceId],
  )

  const createSpace = useCallback(
    (name: string, emoji?: string): Space => {
      const ts = now()
      const space: Space = {
        id: crypto.randomUUID(),
        name: name.trim() || "Untitled",
        emoji: emoji ?? "\u{1F680}",
        createdAt: ts,
        updatedAt: ts,
      }
      const next = [...spaces, space]
      setSpaces(next)
      setActiveSpaceId(space.id)
      persist(next, space.id)
      return space
    },
    [spaces],
  )

  const deleteSpace = useCallback(
    (id: string) => {
      if (spaces.length <= 1) return // keep at least one space
      const next = spaces.filter((s) => s.id !== id)
      const nextActive = id === activeSpaceId ? next[0].id : activeSpaceId
      setSpaces(next)
      setActiveSpaceId(nextActive)
      persist(next, nextActive)
    },
    [spaces, activeSpaceId],
  )

  const renameSpace = useCallback(
    (id: string, name: string) => {
      const next = spaces.map((s) =>
        s.id === id ? { ...s, name: name.trim() || s.name, updatedAt: now() } : s,
      )
      setSpaces(next)
      localStorage.setItem(SPACES_KEY, JSON.stringify(next))
    },
    [spaces],
  )

  const setActiveSpace = useCallback(
    (id: string) => {
      if (spaces.some((s) => s.id === id)) {
        setActiveSpaceId(id)
        localStorage.setItem(ACTIVE_KEY, id)
      }
    },
    [spaces],
  )

  const value = useMemo<SpaceContextValue>(
    () => ({
      spaces,
      activeSpaceId,
      activeSpace,
      createSpace,
      deleteSpace,
      renameSpace,
      setActiveSpace,
    }),
    [spaces, activeSpaceId, activeSpace, createSpace, deleteSpace, renameSpace, setActiveSpace],
  )

  return <SpaceContext.Provider value={value}>{children}</SpaceContext.Provider>
}

export function useSpaces(): SpaceContextValue {
  const ctx = useContext(SpaceContext)
  if (!ctx) {
    throw new Error("useSpaces must be used within a SpaceProvider")
  }
  return ctx
}
