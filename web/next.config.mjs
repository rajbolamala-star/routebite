/** @type {import('next').NextConfig} */
const nextConfig = {
  async rewrites() {
    const apiBase = process.env.ROUTEBITE_API_BASE || "http://localhost:8080";
    return [
      {
        source: "/v1/:path*",
        destination: `${apiBase}/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
