const API = import.meta.env.VITE_API_URL || "http://localhost:8003";

let _getRefreshToken = null;
let _saveTokens = null;
let _onLogout = null;
let _refreshing = null;

export function setAuthCallbacks({ getRefreshToken, saveTokens, onLogout }) {
  _getRefreshToken = getRefreshToken;
  _saveTokens = saveTokens;
  _onLogout = onLogout;
}

async function refreshAccessToken() {
  const rt = _getRefreshToken?.();
  if (!rt) throw { status: 401, detail: "No refresh token" };
  const res = await fetch(`${API}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: rt }),
  });
  if (!res.ok) throw { status: res.status, detail: "Refresh failed" };
  const data = await res.json();
  _saveTokens?.(data.access_token, data.refresh_token);
  return data.access_token;
}

async function request(path, { method = "GET", body, token } = {}) {
  const headers = {};
  if (body) headers["Content-Type"] = "application/json";
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${API}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (res.status === 204) return null;

  if (res.status === 401 && token && _getRefreshToken) {
    try {
      if (!_refreshing) _refreshing = refreshAccessToken();
      const newToken = await _refreshing;
      _refreshing = null;
      const retry = await fetch(`${API}${path}`, {
        method,
        headers: {
          ...headers,
          Authorization: `Bearer ${newToken}`,
        },
        body: body ? JSON.stringify(body) : undefined,
      });
      if (retry.status === 204) return null;
      const retryData = await retry.json();
      if (!retry.ok) throw { status: retry.status, detail: retryData.detail || "Request failed" };
      return retryData;
    } catch {
      _refreshing = null;
      _onLogout?.();
      throw { status: 401, detail: "Session expired" };
    }
  }

  const data = await res.json();
  if (!res.ok) throw { status: res.status, detail: data.detail || "Request failed" };
  return data;
}

// Auth
export const register = (username, email, password) =>
  request("/auth/register", { method: "POST", body: { username, email, password } });

export const login = (username, password) =>
  request("/auth/login", { method: "POST", body: { username, password } });

export const refresh = (refresh_token) =>
  request("/auth/refresh", { method: "POST", body: { refresh_token } });

// Rooms
export const listRooms = (token) =>
  request("/rooms", { token });

export const createRoom = (name, member_ids, token) =>
  request("/rooms", { method: "POST", body: { name, member_ids }, token });

export const getRoom = (roomId, token) =>
  request(`/rooms/${roomId}`, { token });

export const getMembers = (roomId, token) =>
  request(`/rooms/${roomId}/members`, { token });

export const addMembers = (roomId, usernames, token) =>
  request(`/rooms/${roomId}/members`, { method: "POST", body: { usernames }, token });

export const leaveRoom = (roomId, token) =>
  request(`/rooms/${roomId}/members`, { method: "DELETE", token });

// Messages
export const getMessages = (roomId, token, limit = 50) =>
  request(`/rooms/${roomId}/messages?limit=${limit}`, { token });

// Unread counts
export const getUnreadCounts = (token) =>
  request("/rooms/unread", { token });

export const ackUnread = (roomId, token) =>
  request(`/rooms/${roomId}/ack`, { method: "POST", token });

// Friends
export const listFriends = (token) =>
  request("/friends", { token });

export const listFriendRequests = (token) =>
  request("/friends/requests", { token });

export const searchUsers = (q, token) =>
  request(`/friends/search?q=${encodeURIComponent(q)}`, { token });

export const sendFriendRequest = (username, token) =>
  request("/friends/request", { method: "POST", body: { username }, token });

export const acceptFriendRequest = (username, token) =>
  request("/friends/accept", { method: "POST", body: { username }, token });

export const removeFriend = (username, token) =>
  request(`/friends/${encodeURIComponent(username)}`, { method: "DELETE", token });
