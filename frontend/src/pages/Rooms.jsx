import { useState, useEffect, useRef, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { useWebSocket } from "../useWebSocket";
import {
  listRooms, createRoom, getMessages, getMembers, leaveRoom, addMembers,
  getUnreadCounts, ackUnread,
  listFriends, listFriendRequests, searchUsers, sendFriendRequest, acceptFriendRequest, removeFriend,
  refresh,
} from "../api";

export default function Rooms() {
  const { accessToken, user, logout, saveTokens } = useAuth();
  const { roomId: urlRoomId } = useParams();
  const navigate = useNavigate();
  const [rooms, setRooms] = useState([]);
  const selectedRoom = urlRoomId || null;
  const [messages, setMessages] = useState([]);
  const [members, setMembers] = useState([]);
  const [addMemberId, setAddMemberId] = useState("");
  const [error, setError] = useState("");
  const [msgInput, setMsgInput] = useState("");
  const [unreadCounts, setUnreadCounts] = useState({});
  const messagesEndRef = useRef(null);
  const selectedRoomRef = useRef(selectedRoom);

  // Sidebar tab: "rooms" or "friends"
  const [sidebarTab, setSidebarTab] = useState("rooms");

  // Friends state
  const [friends, setFriends] = useState([]);
  const [friendRequests, setFriendRequests] = useState([]);
  const [friendSearch, setFriendSearch] = useState("");
  const [friendSearchResults, setFriendSearchResults] = useState([]);

  // Create room modal
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newRoomName, setNewRoomName] = useState("");
  const [createSearch, setCreateSearch] = useState("");
  const [createSearchResults, setCreateSearchResults] = useState([]);
  const [selectedFriends, setSelectedFriends] = useState([]);

  useEffect(() => { selectedRoomRef.current = selectedRoom; }, [selectedRoom]);

  const handleWsMessage = useCallback((msg) => {
    if (msg.type === "new_message") {
      // If this is our own echo, replace the oldest pending optimistic message
      // for this room (FIFO). Works regardless of which room is selected.
      if (msg.sender_id === user?.id) {
        setMessages(prev => {
          const idx = prev.findIndex(
            m => m._status === "pending" && m.room_id === msg.room_id
          );
          if (idx !== -1) {
            const updated = [...prev];
            updated[idx] = msg;
            return updated;
          }
          // No pending match — append only if viewing this room
          if (msg.room_id === selectedRoomRef.current) {
            return [...prev, msg];
          }
          return prev;
        });
      } else if (msg.room_id === selectedRoomRef.current) {
        setMessages(prev => [...prev, msg]);
      }

      // Update unread counts for messages in other rooms
      if (msg.room_id === selectedRoomRef.current) {
        ackUnread(msg.room_id, accessToken).catch(() => {});
      } else {
        setUnreadCounts(prev => ({
          ...prev,
          [msg.room_id]: (prev[msg.room_id] || 0) + 1,
        }));
      }
    } else if (msg.type === "error" && msg.nonce) {
      setMessages(prev => prev.map(m =>
        m._nonce === msg.nonce && m._status === "pending"
          ? { ...m, _status: "failed" }
          : m
      ));
    }
  }, [accessToken, user]);

  const handleTokenExpired = useCallback(async () => {
    const rt = localStorage.getItem("refresh_token");
    if (!rt) { logout(); return; }
    try {
      const data = await refresh(rt);
      saveTokens(data.access_token, data.refresh_token);
    } catch {
      logout();
    }
  }, [saveTokens, logout]);

  const { status: wsStatus, sendMessage } = useWebSocket(accessToken, handleWsMessage, handleTokenExpired);

  // Auto-fail pending messages after 10 seconds with no echo
  useEffect(() => {
    const interval = setInterval(() => {
      const cutoff = Date.now() - 10_000;
      setMessages(prev => {
        const hasStale = prev.some(m => m._status === "pending" && new Date(m.created_at).getTime() < cutoff);
        if (!hasStale) return prev;
        return prev.map(m =>
          m._status === "pending" && new Date(m.created_at).getTime() < cutoff
            ? { ...m, _status: "failed" }
            : m
        );
      });
    }, 3000);
    return () => clearInterval(interval);
  }, []);

  async function loadRooms() {
    try {
      const data = await listRooms(accessToken);
      setRooms(data);
    } catch {
      setError("Failed to load rooms");
    }
  }

  async function loadUnreadCounts() {
    try {
      const data = await getUnreadCounts(accessToken);
      setUnreadCounts(data.counts || {});
    } catch {
      // best-effort
    }
  }

  async function loadMessages(roomId) {
    try {
      const data = await getMessages(roomId, accessToken);
      setMessages(data);
    } catch {
      setMessages([]);
    }
  }

  async function loadMembers(roomId) {
    try {
      const data = await getMembers(roomId, accessToken);
      setMembers(data);
    } catch {
      setMembers([]);
    }
  }

  useEffect(() => {
    loadRooms();
    loadUnreadCounts();
    loadFriendRequests();
  }, []);

  useEffect(() => {
    if (selectedRoom) {
      loadMessages(selectedRoom);
      loadMembers(selectedRoom);
      setUnreadCounts(prev => {
        const next = { ...prev };
        delete next[selectedRoom];
        return next;
      });
      ackUnread(selectedRoom, accessToken).catch(() => {});
    }
  }, [selectedRoom]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  async function handleCreateRoom(e) {
    e.preventDefault();
    setError("");
    try {
      await createRoom(newRoomName, selectedFriends.map(f => f.id), accessToken);
      setNewRoomName("");
      setSelectedFriends([]);
      setCreateSearch("");
      setCreateSearchResults([]);
      setShowCreateModal(false);
      loadRooms();
    } catch (err) {
      setError(err.detail || "Failed to create room");
    }
  }

  async function handleLeaveRoom(roomId) {
    try {
      await leaveRoom(roomId, accessToken);
      if (selectedRoom === roomId) {
        navigate("/");
        setMessages([]);
        setMembers([]);
      }
      loadRooms();
    } catch (err) {
      setError(err.detail || "Failed to leave room");
    }
  }

  async function handleAddMember(e) {
    e.preventDefault();
    if (!addMemberId.trim() || !selectedRoom) return;
    try {
      await addMembers(selectedRoom, [addMemberId.trim()], accessToken);
      setAddMemberId("");
      loadMembers(selectedRoom);
    } catch (err) {
      setError(err.detail || "User not found");
    }
  }

  const selectedRoomObj = rooms.find(r => r.id === selectedRoom);

  // --- Friends functions ---
  async function loadFriends() {
    try {
      const data = await listFriends(accessToken);
      setFriends(data);
    } catch { /* best-effort */ }
  }

  async function loadFriendRequests() {
    try {
      const data = await listFriendRequests(accessToken);
      setFriendRequests(data);
    } catch { /* best-effort */ }
  }

  useEffect(() => {
    if (sidebarTab === "friends") {
      loadFriends();
      loadFriendRequests();
    }
  }, [sidebarTab]);

  useEffect(() => {
    if (!friendSearch.trim()) { setFriendSearchResults([]); return; }
    const timeout = setTimeout(async () => {
      try {
        const results = await searchUsers(friendSearch.trim(), accessToken);
        setFriendSearchResults(results);
      } catch { setFriendSearchResults([]); }
    }, 300);
    return () => clearTimeout(timeout);
  }, [friendSearch]);

  useEffect(() => {
    if (!createSearch.trim()) { setCreateSearchResults([]); return; }
    const timeout = setTimeout(async () => {
      try {
        const results = await listFriends(accessToken);
        setCreateSearchResults(results.filter(f =>
          f.username.toLowerCase().includes(createSearch.trim().toLowerCase()) &&
          !selectedFriends.some(s => s.id === f.id)
        ));
      } catch { setCreateSearchResults([]); }
    }, 300);
    return () => clearTimeout(timeout);
  }, [createSearch, selectedFriends]);

  async function handleSendFriendRequest(username) {
    try {
      await sendFriendRequest(username, accessToken);
      setFriendSearch("");
      setFriendSearchResults([]);
    } catch (err) {
      setError(err.detail || "Failed to send request");
    }
  }

  async function handleAcceptRequest(username) {
    try {
      await acceptFriendRequest(username, accessToken);
      loadFriends();
      loadFriendRequests();
    } catch (err) {
      setError(err.detail || "Failed to accept request");
    }
  }

  async function handleRemoveFriend(username) {
    try {
      await removeFriend(username, accessToken);
      loadFriends();
      loadFriendRequests();
    } catch (err) {
      setError(err.detail || "Failed to remove friend");
    }
  }

  function handleRetry(msg) {
    const nonce = crypto.randomUUID();
    setMessages(prev => prev.map(m =>
      m._nonce === msg._nonce
        ? { ...m, _status: "pending", _nonce: nonce, message_id: nonce }
        : m
    ));
    sendMessage(msg.room_id, msg.content, nonce);
  }

  const wsColor = wsStatus === "connected" ? "var(--online)" : wsStatus === "connecting" ? "var(--idle)" : "var(--offline)";

  return (
    <>
    <div className="discord-app">
      {/* Channel sidebar */}
      <div className="channel-sidebar">
        <div className="sidebar-header">Chat Rooms</div>

        {/* Sidebar tabs */}
        <div className="sidebar-tabs">
          <button className={`sidebar-tab${sidebarTab === "rooms" ? " active" : ""}`} onClick={() => setSidebarTab("rooms")}>Rooms</button>
          <button className={`sidebar-tab${sidebarTab === "friends" ? " active" : ""}`} onClick={() => setSidebarTab("friends")}>
            Friends{friendRequests.length > 0 && <span className="tab-badge">{friendRequests.length}</span>}
          </button>
        </div>

        {sidebarTab === "rooms" ? (
          <>
            <div className="sidebar-section-title">
              Rooms
              <button className="btn-create-room" onClick={() => setShowCreateModal(true)} title="Create Room">+</button>
            </div>
            <button className="btn-create-room-full" onClick={() => setShowCreateModal(true)}>Create Room</button>

            <div className="room-list">
              {rooms.map(r => (
                <div
                  key={r.id}
                  onClick={() => navigate(`/rooms/${r.id}`)}
                  className={`room-item${selectedRoom === r.id ? " selected" : ""}`}
                >
                  <span className="room-name">{r.name}</span>
                  {unreadCounts[r.id] > 0 && (
                    <span className="unread-badge">{unreadCounts[r.id]}</span>
                  )}
                  <button
                    className="btn-danger"
                    onClick={e => { e.stopPropagation(); handleLeaveRoom(r.id); }}
                  >
                    ✕
                  </button>
                </div>
              ))}
            </div>
          </>
        ) : (
          <div className="friends-panel">
            {/* Search users to add */}
            <div className="friends-search">
              <input
                placeholder="Search users to add..."
                value={friendSearch}
                onChange={e => setFriendSearch(e.target.value)}
              />
              {friendSearchResults.length > 0 && (
                <div className="friends-search-results">
                  {friendSearchResults.map(u => (
                    <div key={u.id} className="friend-search-item">
                      <span>{u.username}</span>
                      <button className="btn-secondary" onClick={() => handleSendFriendRequest(u.username)}>Add</button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Incoming requests */}
            {friendRequests.length > 0 && (
              <>
                <div className="sidebar-section-title">Incoming Requests</div>
                <div className="friends-list">
                  {friendRequests.map(r => (
                    <div key={r.id} className="friend-item">
                      <span className="friend-name">{r.username}</span>
                      <div className="friend-actions">
                        <button className="btn-accept" onClick={() => handleAcceptRequest(r.username)}>✓</button>
                        <button className="btn-danger" onClick={() => handleRemoveFriend(r.username)}>✕</button>
                      </div>
                    </div>
                  ))}
                </div>
              </>
            )}

            {/* Friends list */}
            <div className="sidebar-section-title">Friends — {friends.length}</div>
            <div className="friends-list">
              {friends.map(f => (
                <div key={f.id} className="friend-item">
                  <span className="friend-name">{f.username}</span>
                  <button className="btn-danger" onClick={() => handleRemoveFriend(f.username)} title="Remove friend">✕</button>
                </div>
              ))}
              {friends.length === 0 && <div className="empty-friends">No friends yet</div>}
            </div>
          </div>
        )}

        {/* User panel */}
        <div className="user-panel">
          <div className="user-info">
            <div className="user-avatar">
              <img src="/avatar.svg" alt="avatar" className="avatar-img" />
              <span className="status-dot" style={{ background: wsColor }} />
            </div>
            <div>
              <div className="user-name">{user?.username}</div>
              <div className="user-status">{wsStatus === "connected" ? "Online" : wsStatus === "connecting" ? "Connecting..." : "Offline"}</div>
            </div>
          </div>
          <button className="btn-danger" onClick={logout} style={{ fontSize: 14 }} title="Logout">⏻</button>
        </div>
      </div>

      {/* Chat area */}
      <div className="chat-area">
        {error && <div className="error-banner">{error}</div>}

        {selectedRoomObj ? (
          <>
            {/* Chat header */}
            <div className="chat-header">
              <span>{selectedRoomObj.name}</span>
              <span className="divider" />
              <span className="members-info">{members.length} member{members.length !== 1 ? "s" : ""}</span>
            </div>

            {/* Members bar */}
            <div className="members-bar">
              <span>{members.map(m => m.username).join(", ")}</span>
              <form onSubmit={handleAddMember} style={{ display: "flex", gap: 4, marginLeft: "auto" }}>
                <input
                  placeholder="Add friend by username"
                  value={addMemberId}
                  onChange={e => setAddMemberId(e.target.value)}
                />
                <button className="btn-secondary" type="submit">Add</button>
              </form>
            </div>

            {/* Messages */}
            <div className="messages-container">
              {messages.length === 0 ? (
                <div className="empty-state">
                  <div style={{ textAlign: "center" }}>
                    <div style={{ fontSize: 40, marginBottom: 8 }}>💬</div>
                    <div style={{ fontWeight: 600, fontSize: 18, color: "var(--text-primary)", marginBottom: 4 }}>Welcome to {selectedRoomObj.name}</div>
                    <div>This is the start of the conversation.</div>
                  </div>
                </div>
              ) : (
                messages.map((m, i) => {
                  const sender = members.find(mem => mem.id === m.sender_id);
                  const displayName = sender?.username || m.sender_name || m.sender_id?.slice(0, 8);
                  const prevMsg = messages[i - 1];
                  const isGroupStart = !prevMsg || prevMsg.sender_id !== m.sender_id;
                  const isFailed = m._status === "failed";
                  const isPending = m._status === "pending";
                  const statusClass = isFailed ? " message-failed" : isPending ? " message-pending" : "";

                  const failedIndicator = isFailed && (
                    <div className="msg-failed-row">
                      <span className="msg-failed-icon" title="We were unable to deliver your message, try again later.">⚠</span>
                      <button className="msg-retry-btn" onClick={() => handleRetry(m)}>Retry</button>
                    </div>
                  );

                  return isGroupStart ? (
                    <div key={m._nonce || m.message_id} className={`message message-group-start${statusClass}`}>
                      <div className="msg-avatar"><img src="/avatar.svg" alt="avatar" className="avatar-img" /></div>
                      <div className="msg-body">
                        <div className="msg-header">
                          <span className="msg-author">{displayName}</span>
                          <span className="msg-timestamp">{new Date(m.created_at).toLocaleString()}</span>
                        </div>
                        <div className="msg-content">{m.content}</div>
                        {failedIndicator}
                      </div>
                    </div>
                  ) : (
                    <div key={m._nonce || m.message_id} className={`message${statusClass}`} style={{ paddingLeft: 56 }}>
                      <div className="msg-body">
                        <div className="msg-content">{m.content}</div>
                        {failedIndicator}
                      </div>
                    </div>
                  );
                })
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* Message input */}
            <div className="chat-input-container">
              <form
                onSubmit={e => {
                  e.preventDefault();
                  if (!msgInput.trim()) return;
                  const content = msgInput.trim();
                  const nonce = crypto.randomUUID();
                  setMessages(prev => [...prev, {
                    _nonce: nonce,
                    _status: "pending",
                    message_id: nonce,
                    room_id: selectedRoom,
                    sender_id: user.id,
                    sender_name: user.username,
                    content,
                    created_at: new Date().toISOString(),
                  }]);
                  const sent = sendMessage(selectedRoom, content, nonce);
                  if (!sent) {
                    setMessages(prev => prev.map(m =>
                      m._nonce === nonce ? { ...m, _status: "failed" } : m
                    ));
                  }
                  setMsgInput("");
                }}
                className="chat-input-wrapper"
              >
                <input
                  value={msgInput}
                  onChange={e => setMsgInput(e.target.value)}
                  placeholder={wsStatus === "connected" ? `Message ${selectedRoomObj.name}` : "Connecting..."}
                  disabled={wsStatus !== "connected"}
                />
                <button type="submit" disabled={wsStatus !== "connected"}>➤</button>
              </form>
            </div>
          </>
        ) : (
          <div className="empty-state">
            <div style={{ textAlign: "center" }}>
              <div style={{ fontSize: 48, marginBottom: 16 }}>💬</div>
              <div style={{ fontWeight: 600, fontSize: 20, color: "var(--text-primary)", marginBottom: 8 }}>Select a room</div>
              <div>Pick a room from the sidebar to start chatting.</div>
            </div>
          </div>
        )}
      </div>
    </div>

    {/* Create room modal */}
    {showCreateModal && (
      <div className="modal-overlay" onClick={() => setShowCreateModal(false)}>
        <div className="modal" onClick={e => e.stopPropagation()}>
          <div className="modal-header">
            <span>Create Room</span>
            <button className="btn-danger" onClick={() => setShowCreateModal(false)}>✕</button>
          </div>
          <form onSubmit={handleCreateRoom}>
            <input
              className="modal-input"
              placeholder="Room name"
              value={newRoomName}
              onChange={e => setNewRoomName(e.target.value)}
              required
              autoFocus
            />
            <input
              className="modal-input"
              placeholder="Search friends to add..."
              value={createSearch}
              onChange={e => setCreateSearch(e.target.value)}
            />
            {createSearchResults.length > 0 && (
              <div className="modal-search-results">
                {createSearchResults.map(f => (
                  <div key={f.id} className="friend-search-item" onClick={() => {
                    setSelectedFriends(prev => [...prev, f]);
                    setCreateSearch("");
                    setCreateSearchResults([]);
                  }}>
                    <span>{f.username}</span>
                    <span style={{ color: "var(--text-muted)", fontSize: 12 }}>click to add</span>
                  </div>
                ))}
              </div>
            )}
            {selectedFriends.length > 0 && (
              <div className="selected-friends">
                {selectedFriends.map(f => (
                  <span key={f.id} className="selected-friend-chip">
                    {f.username}
                    <button type="button" onClick={() => setSelectedFriends(prev => prev.filter(s => s.id !== f.id))}>✕</button>
                  </span>
                ))}
              </div>
            )}
            <button className="modal-submit" type="submit">Create</button>
          </form>
        </div>
      </div>
    )}
    </>
  );
}
