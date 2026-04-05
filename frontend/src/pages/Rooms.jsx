import { useState, useEffect, useRef } from "react";
import { useAuth } from "../auth";
import { listRooms, createRoom, getMessages, getMembers, leaveRoom, addMembers } from "../api";

export default function Rooms() {
  const { accessToken, user, logout } = useAuth();
  const [rooms, setRooms] = useState([]);
  const [selectedRoom, setSelectedRoom] = useState(null);
  const [messages, setMessages] = useState([]);
  const [members, setMembers] = useState([]);
  const [newRoomName, setNewRoomName] = useState("");
  const [newMemberIds, setNewMemberIds] = useState("");
  const [addMemberId, setAddMemberId] = useState("");
  const [error, setError] = useState("");
  const messagesEndRef = useRef(null);

  useEffect(() => {
    loadRooms();
  }, []);

  useEffect(() => {
    if (selectedRoom) {
      loadMessages(selectedRoom);
      loadMembers(selectedRoom);
    }
  }, [selectedRoom]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  async function loadRooms() {
    try {
      const data = await listRooms(accessToken);
      setRooms(data);
    } catch {
      setError("Failed to load rooms");
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

  async function handleCreateRoom(e) {
    e.preventDefault();
    setError("");
    try {
      const memberIds = newMemberIds
        .split(",")
        .map(s => s.trim())
        .filter(Boolean);
      await createRoom(newRoomName, memberIds, accessToken);
      setNewRoomName("");
      setNewMemberIds("");
      loadRooms();
    } catch (err) {
      setError(err.detail || "Failed to create room");
    }
  }

  async function handleLeaveRoom(roomId) {
    try {
      await leaveRoom(roomId, accessToken);
      if (selectedRoom === roomId) {
        setSelectedRoom(null);
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

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      {/* Sidebar */}
      <div style={{ width: 260, borderRight: "1px solid #ccc", padding: 12, overflowY: "auto" }}>
        <div style={{ marginBottom: 12 }}>
          <strong>{user?.username}</strong>
          <button onClick={logout} style={{ marginLeft: 8, fontSize: 12 }}>Logout</button>
        </div>

        <h3 style={{ margin: "8px 0" }}>Rooms</h3>
        {rooms.map(r => (
          <div
            key={r.id}
            onClick={() => setSelectedRoom(r.id)}
            style={{
              padding: "6px 8px",
              cursor: "pointer",
              background: selectedRoom === r.id ? "#e0e0ff" : "transparent",
              borderRadius: 4,
              marginBottom: 2,
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
            }}
          >
            <span>{r.name}</span>
            <button
              onClick={e => { e.stopPropagation(); handleLeaveRoom(r.id); }}
              style={{ fontSize: 10, padding: "2px 4px" }}
            >
              Leave
            </button>
          </div>
        ))}

        <hr />
        <form onSubmit={handleCreateRoom}>
          <input
            placeholder="Room name"
            value={newRoomName}
            onChange={e => setNewRoomName(e.target.value)}
            required
            style={{ width: "100%", marginBottom: 4 }}
          />
          <input
            placeholder="Member UUIDs (comma-sep)"
            value={newMemberIds}
            onChange={e => setNewMemberIds(e.target.value)}
            style={{ width: "100%", marginBottom: 4, fontSize: 11 }}
          />
          <button type="submit" style={{ width: "100%" }}>Create Room</button>
        </form>
      </div>

      {/* Main area */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column", padding: 12 }}>
        {error && <p style={{ color: "red" }}>{error}</p>}

        {selectedRoomObj ? (
          <>
            <h2 style={{ margin: 0 }}>{selectedRoomObj.name}</h2>

            {/* Members bar */}
            <div style={{ fontSize: 12, color: "#666", marginBottom: 8 }}>
              Members: {members.map(m => m.username).join(", ")}
              <form onSubmit={handleAddMember} style={{ display: "inline", marginLeft: 12 }}>
                <input
                  placeholder="Add by username"
                  value={addMemberId}
                  onChange={e => setAddMemberId(e.target.value)}
                  style={{ fontSize: 11, width: 220 }}
                />
                <button type="submit" style={{ fontSize: 11 }}>Add</button>
              </form>
            </div>

            {/* Messages */}
            <div style={{ flex: 1, overflowY: "auto", border: "1px solid #ddd", borderRadius: 4, padding: 8 }}>
              {messages.length === 0 ? (
                <p style={{ color: "#999" }}>No messages yet.</p>
              ) : (
                messages.map(m => {
                  const sender = members.find(mem => mem.id === m.sender_id);
                  return (
                    <div key={m.message_id} style={{ marginBottom: 8 }}>
                      <strong>{sender?.username || m.sender_id.slice(0, 8)}</strong>
                      <span style={{ fontSize: 11, color: "#999", marginLeft: 8 }}>
                        {new Date(m.created_at).toLocaleString()}
                      </span>
                      <div>{m.content}</div>
                    </div>
                  );
                })
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* Note: no send message yet — that comes with WebSocket in Phase 4 */}
            <p style={{ fontSize: 11, color: "#999", margin: "4px 0 0" }}>
              Sending messages requires WebSocket (Phase 4).
            </p>
          </>
        ) : (
          <p style={{ color: "#999", marginTop: 40 }}>Select a room from the sidebar.</p>
        )}
      </div>
    </div>
  );
}
