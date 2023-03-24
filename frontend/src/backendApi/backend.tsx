export function GET(uri: string, options?: RequestInit): Promise<any> {
  return fetch(process.env.REACT_APP_BACKEND_URL + "/api/v1" + uri, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
    },
    body: options?.body,
    cache: options?.cache,
  });
}

export function POST(uri: string, options?: RequestInit): Promise<any> {
  return fetch(process.env.REACT_APP_BACKEND_URL + "/api/v1" + uri, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: options?.body,
    cache: options?.cache,
  });
}
