import React from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';
import { ToolList } from './pages/ToolList';

const rootElement = document.getElementById('root');

if (rootElement === null) {
  throw new Error('Root element not found');
}

createRoot(rootElement).render(
  <React.StrictMode>
    <ToolList />
  </React.StrictMode>,
);
