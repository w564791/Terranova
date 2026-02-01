import { configureStore } from '@reduxjs/toolkit';
import authSlice from './slices/authSlice';
import moduleSlice from './slices/moduleSlice';
import workspaceSlice from './slices/workspaceSlice';

export const store = configureStore({
  reducer: {
    auth: authSlice,
    modules: moduleSlice,
    workspaces: workspaceSlice,
  },
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;