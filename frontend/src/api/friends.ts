import { apiClient } from './client';
import { FriendUser, FriendRequest, FriendRequestsResponse } from './types';

export async function getFriends(): Promise<FriendUser[]> {
  return apiClient.get<FriendUser[]>('/friends');
}

export async function getFriendRequests(): Promise<FriendRequestsResponse> {
  return apiClient.get<FriendRequestsResponse>('/friend-requests');
}

export async function sendFriendRequest(username: string): Promise<FriendRequest> {
  return apiClient.post<FriendRequest>('/friend-requests', { username });
}

export async function acceptFriendRequest(requestId: string): Promise<FriendUser> {
  return apiClient.post<FriendUser>(`/friend-requests/${requestId}/accept`);
}

export async function declineOrCancelRequest(requestId: string): Promise<void> {
  return apiClient.delete(`/friend-requests/${requestId}`);
}

export async function removeFriend(userId: string): Promise<void> {
  return apiClient.delete(`/friends/${userId}`);
}
