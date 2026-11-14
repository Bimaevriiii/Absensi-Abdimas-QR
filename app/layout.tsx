import type { Metadata } from 'next';
import './globals.css';
export const metadata: Metadata = { title:'Hadir — Absensi Sekolah', description:'Satu gerbang, semua kehadiran. Absensi QR untuk murid dan guru.'};
export default function RootLayout({children}:{children:React.ReactNode}) {return <html lang="id"><body>{children}</body></html>}
