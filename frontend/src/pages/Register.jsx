import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { register as apiRegister, login as apiLogin } from "../api";
import { useAuth } from "../auth";

export default function Register() {
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const { saveTokens } = useAuth();
  const navigate = useNavigate();

  async function handleSubmit(e) {
    e.preventDefault();
    setError("");
    try {
      await apiRegister(username, email, password);
      const data = await apiLogin(username, password);
      saveTokens(data.access_token, data.refresh_token);
      navigate("/");
    } catch (err) {
      setError(err.detail || "Registration failed");
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h2>Create an account</h2>
        <p className="subtitle">Join the conversation</p>
        <form onSubmit={handleSubmit}>
          <label>Username</label>
          <input value={username} onChange={e => setUsername(e.target.value)} required />
          <label>Email</label>
          <input type="email" value={email} onChange={e => setEmail(e.target.value)} required />
          <label>Password</label>
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required />
          {error && <p className="error-msg">{error}</p>}
          <button type="submit">Register</button>
          <p className="auth-footer">Already have an account? <Link to="/login">Log In</Link></p>
        </form>
      </div>
    </div>
  );
}
