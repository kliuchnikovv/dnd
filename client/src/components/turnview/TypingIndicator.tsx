import React, { useEffect, useRef } from 'react';
import { Animated, StyleSheet, View } from 'react-native';

// TypingIndicator — «Мастер печатает»: три точки с бегущей волной прозрачности.
// Показывается, пока проза ещё не пошла (пустой streaming-блок), как в nomi.
// Волну даёт сдвиг фазы по индексу; период ~1.1с — тот же ритм, что StreamingCursor.
export const TypingIndicator: React.FC<{ color: string }> = ({ color }) => {
    const dots = [useRef(new Animated.Value(0.3)).current, useRef(new Animated.Value(0.3)).current, useRef(new Animated.Value(0.3)).current];
    useEffect(() => {
        const loops = dots.map((v, i) =>
            Animated.loop(
                Animated.sequence([
                    Animated.delay(i * 180),
                    Animated.timing(v, { toValue: 1, duration: 380, useNativeDriver: true }),
                    Animated.timing(v, { toValue: 0.3, duration: 380, useNativeDriver: true }),
                    Animated.delay((2 - i) * 180),
                ]),
            ),
        );
        loops.forEach((l) => l.start());
        return () => loops.forEach((l) => l.stop());
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    return (
        <View style={styles.row} accessibilityLabel="Мастер печатает">
            {dots.map((v, i) => (
                <Animated.View key={i} style={[styles.dot, { backgroundColor: color, opacity: v }]} />
            ))}
        </View>
    );
};

const styles = StyleSheet.create({
    row: { flexDirection: 'row', alignItems: 'center', gap: 5, paddingVertical: 4 },
    dot: { width: 6, height: 6, borderRadius: 3 },
});
