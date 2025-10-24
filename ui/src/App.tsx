import { BrowserRouter as Router, Route, Routes } from 'react-router-dom';
import './App.css';
import Toolbar from './components/Toolbar';
import Dashboard from './components/Dashboard';
import Schedule from './components/Schedule';
import Search from './components/Search';

function App() {
  return (
    <Router>
      <Toolbar />
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/schedule" element={<Schedule />} />
        <Route path="/search" element={<Search />} />
      </Routes>
    </Router>
  );
}

export default App;
