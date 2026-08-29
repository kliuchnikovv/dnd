import React, { useEffect, useRef } from 'react';
import { Animated } from 'react-native';

// Мигающий курсор у ещё доезжающей прозы. blink ~1.1s (дизайн-хендофф).
export const StreamingCursor: React.FC<{ color: string }> = ({ color }) => {
    const opacity = useRef(new Animated.Value(1)).current;
    useEffect(() => {
        const loop = Animated.loop(
            Animated.sequence([
                Animated.timing(opacity, { toValue: 0, duration: 550, useNativeDriver: true }),
                Animated.timing(opacity, { toValue: 1, duration: 550, useNativeDriver: true }),
            ]),
        );
        loop.start();
        return () => loop.stop();
    }, [opacity]);
    return (
        <Animated.Text style={{ color, opacity, fontFamily: 'IBMPlexMono_700Bold' }}> ▍</Animated.Text>
    );
};
