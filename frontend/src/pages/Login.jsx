import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { login as apiLogin } from "../api";
import { useAuth } from "../auth";

export default function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const { saveTokens } = useAuth();
  const navigate = useNavigate();

  async function handleSubmit(e) {
    e.preventDefault();
    setError("");
    try {
      const data = await apiLogin(username, password);
      saveTokens(data.access_token, data.refresh_token);
      navigate("/");
    } catch (err) {
      setError(err.detail || "Login failed");
    }
  }

  return (
    <div style={{ maxWidth: 360, margin: "80px auto" }}>
      <h2>Login</h2>
      <form onSubmit={handleSubmit}>
        <div>
          <input placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} required />
        </div>
        <div>
          <input placeholder="Password" type="password" value={password} onChange={e => setPassword(e.target.value)} required />
        </div>
        {error && <p style={{ color: "red" }}>{error}</p>}
        <button type="submit">Login</button>
      </form>
      <p>No account? <Link to="/register">Register</Link></p>
    </div>
  );
}
