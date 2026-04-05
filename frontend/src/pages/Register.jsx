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
    <div style={{ maxWidth: 360, margin: "80px auto" }}>
      <h2>Register</h2>
      <form onSubmit={handleSubmit}>
        <div>
          <input placeholder="Username" value={username} onChange={e => setUsername(e.target.value)} required />
        </div>
        <div>
          <input placeholder="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required />
        </div>
        <div>
          <input placeholder="Password" type="password" value={password} onChange={e => setPassword(e.target.value)} required />
        </div>
        {error && <p style={{ color: "red" }}>{error}</p>}
        <button type="submit">Register</button>
      </form>
      <p>Already have an account? <Link to="/login">Login</Link></p>
    </div>
  );
}
