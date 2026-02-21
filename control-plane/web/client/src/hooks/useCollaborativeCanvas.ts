/**
 * useCollaborativeCanvas — bridges Yjs CRDT state ↔ ReactFlow nodes/edges.
 *
 * When connected to a collaboration room:
 *  - Node position changes (drag) are written to Y.Map("positions")
 *  - Agent data mutations are written to Y.Map("agents")
 *  - Edge mutations are written to Y.Array("edges")
 *  - Remote changes from other peers are observed and merged into local ReactFlow state
 *
 * When NOT connected (solo mode), it falls through to plain local state.
 */

import { useCallback, useEffect, useRef } from 'react'
import type { Node, Edge, NodeChange, EdgeChange, Connection } from '@xyflow/react'
import { applyNodeChanges, applyEdgeChanges, addEdge } from '@xyflow/react'
import * as Y from 'yjs'
import { useCollaboration } from '@/contexts/CollaborationContext'
import type { AgentNodeData } from '@/types/agent-canvas'

interface UseCollaborativeCanvasOptions {
  /** Initial local nodes (used in solo mode or to seed the room) */
  initialNodes: Node[]
  /** Initial local edges */
  initialEdges: Edge[]
}

interface UseCollaborativeCanvasReturn {
  /** Apply ReactFlow node changes (from onNodesChange) */
  onNodesChange: (changes: NodeChange[]) => void
  /** Apply ReactFlow edge changes (from onEdgesChange) */
  onEdgesChange: (changes: EdgeChange[]) => void
  /** Handle new connection (from onConnect) */
  onConnect: (connection: Connection) => void
  /** Add a new agent node to the canvas */
  addAgentNode: (node: Node) => void
  /** Remove a node and its edges */
  removeNode: (nodeId: string) => void
  /** Track cursor position for presence */
  onMouseMove: (event: React.MouseEvent) => void
  /** Clear cursor when leaving canvas */
  onMouseLeave: () => void
}

export function useCollaborativeCanvas(
  nodes: Node[],
  setNodes: React.Dispatch<React.SetStateAction<Node[]>>,
  edges: Edge[],
  setEdges: React.Dispatch<React.SetStateAction<Edge[]>>,
  options: UseCollaborativeCanvasOptions,
): UseCollaborativeCanvasReturn {
  const { room, connected, updateCursor } = useCollaboration()
  const isApplyingRemote = useRef(false)

  // -----------------------------------------------------------
  // Sync remote Yjs changes → local ReactFlow state
  // -----------------------------------------------------------
  useEffect(() => {
    if (!room || !connected) return

    const { positions, agents, edges: yEdges } = room

    // Observe position changes from remote peers
    const posObserver = (event: Y.YMapEvent<{ x: number; y: number }>) => {
      if (event.transaction.local) return // skip our own changes
      isApplyingRemote.current = true
      setNodes((prev) =>
        prev.map((node) => {
          const pos = positions.get(node.id)
          if (pos) return { ...node, position: pos }
          return node
        }),
      )
      isApplyingRemote.current = false
    }

    // Observe agent data changes from remote peers
    const agentObserver = (event: Y.YMapEvent<Record<string, unknown>>) => {
      if (event.transaction.local) return
      isApplyingRemote.current = true
      setNodes((prev) =>
        prev.map((node) => {
          if (node.type !== 'agent') return node
          const agentData = agents.get(node.id)
          if (agentData) return { ...node, data: agentData }
          return node
        }),
      )
      isApplyingRemote.current = false
    }

    // Observe edge changes from remote peers
    const edgeObserver = () => {
      isApplyingRemote.current = true
      const remoteEdges: Edge[] = []
      yEdges.forEach((item) => {
        remoteEdges.push(item as unknown as Edge)
      })
      setEdges(remoteEdges)
      isApplyingRemote.current = false
    }

    positions.observe(posObserver)
    agents.observe(agentObserver)
    yEdges.observe(edgeObserver)

    // Seed the room with initial state if it's empty (first joiner)
    if (positions.size === 0 && options.initialNodes.length > 0) {
      room.doc.transact(() => {
        for (const node of options.initialNodes) {
          positions.set(node.id, node.position)
          if (node.type === 'agent' && node.data) {
            agents.set(node.id, node.data as Record<string, unknown>)
          }
        }
        for (const edge of options.initialEdges) {
          yEdges.push([edge as unknown as Record<string, unknown>])
        }
      })
    } else if (positions.size > 0) {
      // Load existing room state into local ReactFlow
      isApplyingRemote.current = true
      setNodes((prev) =>
        prev.map((node) => {
          const pos = positions.get(node.id)
          const agentData = agents.get(node.id)
          return {
            ...node,
            ...(pos ? { position: pos } : {}),
            ...(agentData ? { data: agentData } : {}),
          }
        }),
      )
      const existingEdges: Edge[] = []
      yEdges.forEach((item) => existingEdges.push(item as unknown as Edge))
      if (existingEdges.length > 0) {
        setEdges(existingEdges)
      }
      isApplyingRemote.current = false
    }

    return () => {
      positions.unobserve(posObserver)
      agents.unobserve(agentObserver)
      yEdges.unobserve(edgeObserver)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [room, connected])

  // -----------------------------------------------------------
  // Local ReactFlow changes → write to Yjs
  // -----------------------------------------------------------

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => {
      // Always apply locally
      setNodes((prev) => applyNodeChanges(changes, prev))

      // If connected and this is a local change, propagate positions to Yjs
      if (room && connected && !isApplyingRemote.current) {
        const posChanges = changes.filter(
          (c) => c.type === 'position' && c.position,
        )
        if (posChanges.length > 0) {
          room.doc.transact(() => {
            for (const change of posChanges) {
              if (change.type === 'position' && change.position) {
                room.positions.set(change.id, change.position)
              }
            }
          })
        }
      }
    },
    [room, connected, setNodes],
  )

  const onEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      setEdges((prev) => {
        const next = applyEdgeChanges(changes, prev)

        // Sync full edge list to Yjs
        if (room && connected && !isApplyingRemote.current) {
          room.doc.transact(() => {
            room.edges.delete(0, room.edges.length)
            for (const edge of next) {
              room.edges.push([edge as unknown as Record<string, unknown>])
            }
          })
        }

        return next
      })
    },
    [room, connected, setEdges],
  )

  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((prev) => {
        const next = addEdge(
          { ...connection, style: { stroke: 'var(--border)' } },
          prev,
        )

        if (room && connected && !isApplyingRemote.current) {
          room.doc.transact(() => {
            room.edges.delete(0, room.edges.length)
            for (const edge of next) {
              room.edges.push([edge as unknown as Record<string, unknown>])
            }
          })
        }

        return next
      })
    },
    [room, connected, setEdges],
  )

  const addAgentNode = useCallback(
    (node: Node) => {
      setNodes((prev) => [...prev, node])

      if (room && connected) {
        room.doc.transact(() => {
          room.positions.set(node.id, node.position)
          if (node.type === 'agent' && node.data) {
            room.agents.set(node.id, node.data as Record<string, unknown>)
          }
        })
      }
    },
    [room, connected, setNodes],
  )

  const removeNode = useCallback(
    (nodeId: string) => {
      setNodes((prev) => prev.filter((n) => n.id !== nodeId))
      setEdges((prev) =>
        prev.filter((e) => e.source !== nodeId && e.target !== nodeId),
      )

      if (room && connected) {
        room.doc.transact(() => {
          room.positions.delete(nodeId)
          room.agents.delete(nodeId)
          // Rebuild edges without the deleted node
          const remaining: Record<string, unknown>[] = []
          room.edges.forEach((edge) => {
            const e = edge as unknown as Edge
            if (e.source !== nodeId && e.target !== nodeId) {
              remaining.push(edge)
            }
          })
          room.edges.delete(0, room.edges.length)
          for (const e of remaining) {
            room.edges.push([e])
          }
        })
      }
    },
    [room, connected, setNodes, setEdges],
  )

  // -----------------------------------------------------------
  // Cursor presence
  // -----------------------------------------------------------

  const onMouseMove = useCallback(
    (event: React.MouseEvent) => {
      if (!connected) return
      // Use the canvas-relative position
      const rect = event.currentTarget.getBoundingClientRect()
      updateCursor({
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      })
    },
    [connected, updateCursor],
  )

  const onMouseLeave = useCallback(() => {
    if (!connected) return
    updateCursor(null)
  }, [connected, updateCursor])

  return {
    onNodesChange,
    onEdgesChange,
    onConnect,
    addAgentNode,
    removeNode,
    onMouseMove,
    onMouseLeave,
  }
}
