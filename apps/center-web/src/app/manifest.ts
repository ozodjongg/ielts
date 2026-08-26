import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "IELTS Learning Center",
    short_name: "IELTS Center",
    description: "Secure learning-center administration for Assessment Platform IELTS.",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#ffffff",
  };
}
