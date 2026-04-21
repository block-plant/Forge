// Forge SDK — TypeScript Type Definitions
// Full type coverage for every module in the SDK.

// ---- Core ----

export interface ForgeConfig {
  endpoint: string;
  projectId?: string;
}

export interface ForgeError {
  code: string;
  message: string;
  status: number;
}

// ---- Auth ----

export interface AuthUser {
  uid: string;
  email: string;
  displayName?: string;
  photoURL?: string;
  emailVerified: boolean;
  disabled: boolean;
  customClaims?: Record<string, any>;
  createdAt: string;
  lastLoginAt: string;
}

export interface AuthTokens {
  token: string;
  refreshToken: string;
  expiresIn: number;
  user: AuthUser;
}

export interface SignupRequest {
  email: string;
  password: string;
  displayName?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

// ---- Database ----

export interface DocumentData {
  [key: string]: any;
}

export interface DocumentSnapshot {
  _id: string;
  _created_at: number;
  _updated_at: number;
  _version: number;
  [key: string]: any;
}

export interface QueryFilter {
  field: string;
  operator: "==" | "!=" | ">" | ">=" | "<" | "<=" | "in" | "array-contains";
  value: any;
}

export interface QueryOptions {
  collection: string;
  filters?: QueryFilter[];
  orderBy?: string;
  orderDir?: "asc" | "desc";
  limit?: number;
  offset?: number;
}

export interface QueryResult {
  documents: DocumentSnapshot[];
  total: number;
  hasMore: boolean;
}

export interface ChangeEvent {
  type: "set" | "update" | "delete";
  collection: string;
  document_id: string;
  data?: DocumentData;
  timestamp: number;
}

// ---- Storage ----

export interface FileMetadata {
  name: string;
  path: string;
  size: number;
  contentType: string;
  checksum: string;
  createdAt: string;
  updatedAt: string;
  customMetadata?: Record<string, string>;
}

export interface UploadResult {
  url: string;
  key: string;
  metadata: FileMetadata;
}

// ---- Functions ----

export interface FunctionResult<T = any> {
  data: T;
  status: number;
}

// ---- Analytics ----

export interface AnalyticsEvent {
  name: string;
  properties?: Record<string, any>;
  timestamp?: number;
}

// ---- Realtime ----

export type ConnectionState = "connecting" | "connected" | "disconnected" | "reconnecting";

export interface RealtimeMessage {
  type: string;
  payload: any;
  channel?: string;
  id?: string;
}

export type UnsubscribeFn = () => void;
