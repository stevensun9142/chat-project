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
    <div className="auth-page">
      <div className="auth-card">
        <h2>Welcome back!</h2>
        <p className="subtitle">We're so excited to see you again!</p>
        <form onSubmit={handleSubmit}>
          <label>Username</label>
          <input value={username} onChange={e => setUsername(e.target.value)} required />
          <label>Password</label>
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required />
          {error && <p className="error-msg">{error}</p>}
          <button type="submit">Log In</button>
          <p className="auth-footer">Need an account? <Link to="/register">Register</Link></p>
        </form>
      </div>
    </div>
  );
}
