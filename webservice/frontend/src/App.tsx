import { Routes, Route } from 'react-router-dom'
import TradePage from './pages/Trade'

function App() {
  return (
    <div className="min-h-screen">
      <Routes>
        <Route path="/" element={<TradePage />} />
      </Routes>
    </div>
  )
}

export default App