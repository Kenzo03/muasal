import { ImageResponse } from "next/og";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

// The home-screen icon for iPhone and iPad, which round the corners themselves:
// the sidebar's logo mark in white on a full terracotta square.
export default function AppleIcon() {
  return new ImageResponse(
    (
      <div style={{ width: "100%", height: "100%", display: "flex", alignItems: "center", justifyContent: "center", background: "#b5482a" }}>
        <svg width="112" height="112" viewBox="0 0 16 16" fill="none" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
          <path d="M8 2v5M8 7c-2.8 0-4.5 2-4.5 4.5M8 7c2.8 0 4.5 2 4.5 4.5M3.5 11.5V14M12.5 11.5V14M8 7v7" />
        </svg>
      </div>
    ),
    size,
  );
}
