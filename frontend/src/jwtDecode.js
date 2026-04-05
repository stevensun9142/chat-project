// Minimal JWT decode (no verification — that's the server's job)
export function jwtDecode(token) {
  try {
    const payload = token.split(".")[1];
    const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
    const data = JSON.parse(json);
    return { id: data.sub, username: data.username };
  } catch {
    return null;
  }
}
