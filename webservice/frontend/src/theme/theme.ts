export const theme = {
  radius: { sm: 8, md: 12, lg: 16, pill: 999 },
  shadow: {
    card: '0 8px 30px rgba(0,0,0,.35), 0 0 0 1px #1F2937 inset',
    glow: '0 0 0 1px rgba(139,92,246,.35), 0 10px 40px rgba(139,92,246,.15)',
  },
  spacing: [0,4,8,12,16,20,24,28,32,40,48], // use s[6]=24 as default gap
  font: { 
    family: 'Inter, system-ui, sans-serif', 
    size: { xs:12, sm:14, md:16, lg:18, xl:20, h2:28 } 
  },
  color: {
    bg: { base:'#0B0F1A', card:'#121826', subtle:'#0b1220' }, // dark default
    text: { primary:'#E5E7EB', secondary:'#A3A7B3', muted:'#8B90A0' },
    brand: { primary:'#8B5CF6', accent:'#A78BFA' },
    state: { success:'#22c55e', warn:'#f59e0b', danger:'#ef4444', info:'#60a5fa' },
    border: '#1f2937',
  },
};