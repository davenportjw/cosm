import React, { useState, useEffect } from 'react';

interface User {
  id: string;
  email: string;
}

export const App: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);

  useEffect(() => {
    fetch('/api/v1/users')
      .then(res => res.json())
      .then((data: User[]) => setUsers(data));
  }, []);

  return (
    <div className="container">
      <h1>Future of Git Polyglot App</h1>
      <ul>
        {users.map(u => (
          <li key={u.id}>{u.email}</li>
        ))}
      </ul>
    </div>
  );
};
