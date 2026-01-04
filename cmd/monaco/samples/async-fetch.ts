// Context: API client configuration (Bun only)

interface ApiConfig {
  baseUrl: string;
  timeout: number;
  retryAttempts: number;
  headers: Record<string, string>;
}

export const apiConfig: ApiConfig = {
  baseUrl: "https://jsonplaceholder.typicode.com",
  timeout: 5000,
  retryAttempts: 3,
  headers: {
    'Accept': 'application/json',
    'X-Client-Version': '1.0.0'
  }
};

export function buildUrl(path: string, params?: Record<string, string>): string {
  const url = new URL(path, apiConfig.baseUrl);
  if (params) {
    Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
  }
  return url.toString();
}

export function logApiCall(method: string, path: string, duration: number): void {
  console.log(`[API] ${method} ${path} completed in ${duration}ms`);
}

// --- Code ---

// Async HTTP Fetch - Fetch data from external API (Bun only)
// Demonstrates async/await and HTTP requests

interface Post {
  userId: number;
  id: number;
  title: string;
  body: string;
}

interface Comment {
  postId: number;
  id: number;
  name: string;
  email: string;
  body: string;
}

// Fetch posts from API
async function fetchPosts(limit: number = 5): Promise<Post[]> {
  const start = Date.now();
  const url = buildUrl('/posts');
  
  const response = await fetch(url, { headers: apiConfig.headers });
  const posts: Post[] = await response.json();
  
  logApiCall('GET', '/posts', Date.now() - start);
  return posts.slice(0, limit);
}

// Fetch comments for a specific post
async function fetchComments(postId: number): Promise<Comment[]> {
  const start = Date.now();
  const url = buildUrl(`/posts/${postId}/comments`);
  
  const response = await fetch(url, { headers: apiConfig.headers });
  const comments: Comment[] = await response.json();
  
  logApiCall('GET', `/posts/${postId}/comments`, Date.now() - start);
  return comments;
}

// Post details with comment information
interface PostDetails {
  post: Post;
  commentCount: number;
  commenters: string[];
}

// Fetch post with its comments using parallel requests
async function fetchPostWithComments(postId: number): Promise<PostDetails> {
  const start = Date.now();
  
  const [post, comments] = await Promise.all([
    fetch(buildUrl(`/posts/${postId}`), { headers: apiConfig.headers }).then(r => r.json() as Promise<Post>),
    fetchComments(postId),
  ]);
  
  logApiCall('GET', `/posts/${postId} + comments`, Date.now() - start);
  
  return {
    post,
    commentCount: comments.length,
    commenters: comments.map(c => c.email),
  };
}

// Main execution
async function main() {
  // Fetch some posts
  const posts = await fetchPosts(3);
  
  // Get details for the first post
  const postDetails = await fetchPostWithComments(1);
  
  // Calculate stats
  const totalTitleLength = posts.reduce((acc, p) => acc + p.title.length, 0);
  const avgTitleLength = totalTitleLength / posts.length;
  
  return {
    apiConfiguration: {
      baseUrl: apiConfig.baseUrl,
      timeout: apiConfig.timeout + 'ms',
      retryAttempts: apiConfig.retryAttempts,
    },
    posts: posts.map(p => ({
      id: p.id,
      title: p.title.substring(0, 50) + (p.title.length > 50 ? "..." : ""),
      bodyPreview: p.body.substring(0, 80) + "...",
    })),
    featuredPost: {
      title: postDetails.post.title,
      commentCount: postDetails.commentCount,
      topCommenters: postDetails.commenters.slice(0, 3),
    },
    stats: {
      postsLoaded: posts.length,
      averageTitleLength: Math.round(avgTitleLength),
    },
  };
}

// Execute and export result
export default await main();
