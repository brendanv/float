import { useEffect, useState } from "react";

export function useInitializedState(initialValue, isReady) {
  const [value, setValue] = useState(null);

  useEffect(() => {
    if (isReady && value === null) {
      setValue(initialValue);
    }
  }, [initialValue, isReady, value]);

  return [value, setValue];
}
