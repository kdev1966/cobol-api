import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { FournisseurAuth } from './auth'
import App from './App'
import './styles.css'

const racine = document.getElementById('racine')
if (!racine) throw new Error("element #racine introuvable")

createRoot(racine).render(
  <StrictMode>
    <BrowserRouter>
      <FournisseurAuth>
        <App />
      </FournisseurAuth>
    </BrowserRouter>
  </StrictMode>,
)
